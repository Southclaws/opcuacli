package opc

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// Reader is the subset of the gopcua client the operations in this package use.
// Taking an interface keeps them testable and lets the TUI share one client with
// the commands.
type Reader interface {
	Browse(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error)
	BrowseNext(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error)
	Read(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error)
}

// BrowseOptions controls a walk of the address space.
type BrowseOptions struct {
	// Root is the node the walk starts from; it is not itself reported.
	Root *ua.NodeID
	// Depth is how many levels to descend. 1 lists the immediate children; 0
	// descends without limit.
	Depth int
	// Direction and ReferenceType select which references are followed.
	Direction       ua.BrowseDirection
	ReferenceType   *ua.NodeID
	IncludeSubtypes bool
	// ClassMask filters which node classes are reported. Descent still passes
	// through other classes, so filtering to variables does not hide the
	// folders they live in.
	ClassMask ua.NodeClass
	// Match, when set, reports whether a node should be included.
	Match func(node cligen.Node) bool
	// WithValues reads the Value, DataType and AccessLevel of every variable.
	WithValues bool
	// Limit stops the walk after this many reported nodes; 0 means no limit.
	Limit int
	// Nested builds a tree in the Children field instead of a flat list.
	Nested bool
	// Emit is called for each reported node as it is found, for streaming
	// output.
	Emit func(node cligen.Node) error
}

// Browse walks the address space and returns the nodes found. A flat walk
// returns parents before their children; a nested walk returns the roots of a
// tree, with descendants hanging off Children.
func Browse(ctx context.Context, client Reader, options BrowseOptions) ([]cligen.Node, error) {
	if options.ReferenceType == nil {
		reference, err := ParseReferenceType("HierarchicalReferences")
		if err != nil {
			return nil, err
		}
		options.ReferenceType = reference
	}
	if options.ClassMask == 0 {
		options.ClassMask = ua.NodeClassAll
	}

	walk := &walker{client: client, options: options, visited: map[string]bool{}}

	if options.Nested {
		nodes, err := walk.tree(ctx, options.Root, "", 1)
		if err != nil && !errors.Is(err, errLimitReached) {
			return nil, err
		}
		return nodes, nil
	}

	if err := walk.flat(ctx, options.Root, "", 1); err != nil && !errors.Is(err, errLimitReached) {
		return nil, err
	}
	return walk.found, nil
}

// errLimitReached unwinds a walk that has produced everything it was asked for.
// Browse absorbs it; it is never returned to a caller.
var errLimitReached = errors.New("limit reached")

type walker struct {
	client  Reader
	options BrowseOptions

	// visited guards against the cycles a reference graph can contain: a walk
	// following HasNotifier, or a custom reference, would otherwise revisit the
	// same node for ever.
	visited map[string]bool
	found   []cligen.Node
	count   int
}

// flat walks depth-first, appending every reported node to found.
func (w *walker) flat(ctx context.Context, parent *ua.NodeID, path string, depth int) error {
	level, err := w.level(ctx, parent, path, depth)
	if err != nil {
		return err
	}

	for _, node := range level {
		if w.include(node) {
			w.found = append(w.found, node)
			if err := w.reported(node); err != nil {
				return err
			}
		}

		child, ok := w.next(node, depth)
		if !ok {
			continue
		}
		if err := w.flat(ctx, child, nodePath(node), depth+1); err != nil {
			return err
		}
	}

	return nil
}

// tree walks depth-first, returning each level as the Children of its parent.
func (w *walker) tree(ctx context.Context, parent *ua.NodeID, path string, depth int) ([]cligen.Node, error) {
	level, err := w.level(ctx, parent, path, depth)
	if err != nil {
		return nil, err
	}

	var out []cligen.Node
	for _, node := range level {
		include := w.include(node)
		if include {
			if err := w.reported(node); err != nil {
				return append(out, node), err
			}
		}

		if child, ok := w.next(node, depth); ok {
			children, err := w.tree(ctx, child, nodePath(node), depth+1)
			node.Children = children
			if err != nil {
				// A node excluded by a filter is still worth showing when
				// something under it matched, since a tree without its
				// branches is not a path to anything.
				if include || len(children) > 0 {
					out = append(out, node)
				}
				return out, err
			}
		}

		if include || len(node.Children) > 0 {
			out = append(out, node)
		}
	}

	return out, nil
}

// level browses one node and describes its references, reading the values of any
// variables among them in a single call.
func (w *walker) level(ctx context.Context, parent *ua.NodeID, path string, depth int) ([]cligen.Node, error) {
	references, err := w.references(ctx, parent)
	if err != nil {
		return nil, err
	}

	nodes := make([]cligen.Node, 0, len(references))
	var variables []int
	for _, reference := range references {
		node, ok := describe(reference, path, depth)
		if !ok {
			continue
		}
		nodes = append(nodes, node)
		if reference.NodeClass == ua.NodeClassVariable {
			variables = append(variables, len(nodes)-1)
		}
	}

	if w.options.WithValues && len(variables) > 0 {
		if err := w.readValues(ctx, nodes, variables); err != nil {
			return nil, err
		}
	}

	return nodes, nil
}

// next reports the node to descend into, if the walk should descend at all. It
// marks the node visited, so each node is descended once per walk.
func (w *walker) next(node cligen.Node, depth int) (*ua.NodeID, bool) {
	if w.options.Depth != 0 && depth >= w.options.Depth {
		return nil, false
	}
	if w.visited[node.NodeID] {
		return nil, false
	}
	w.visited[node.NodeID] = true

	target, err := ParseNodeID(node.NodeID)
	if err != nil {
		return nil, false
	}
	return target, true
}

func (w *walker) include(node cligen.Node) bool {
	class := NodeClassFromName(node.NodeClass)
	if w.options.ClassMask != ua.NodeClassAll && class != 0 && w.options.ClassMask&class == 0 {
		return false
	}
	if w.options.Match != nil && !w.options.Match(node) {
		return false
	}
	return true
}

// reported streams a node to the caller and enforces the limit.
func (w *walker) reported(node cligen.Node) error {
	if w.options.Emit != nil {
		if err := w.options.Emit(node); err != nil {
			return err
		}
	}
	w.count++
	if w.options.Limit > 0 && w.count >= w.options.Limit {
		return errLimitReached
	}
	return nil
}

// references calls the Browse service for one node, following continuation
// points until the server has no more references to give.
func (w *walker) references(ctx context.Context, node *ua.NodeID) ([]*ua.ReferenceDescription, error) {
	response, err := w.client.Browse(ctx, &ua.BrowseRequest{
		NodesToBrowse: []*ua.BrowseDescription{{
			NodeID:          node,
			BrowseDirection: w.options.Direction,
			ReferenceTypeID: w.options.ReferenceType,
			IncludeSubtypes: w.options.IncludeSubtypes,
			NodeClassMask:   uint32(ua.NodeClassAll),
			ResultMask:      uint32(ua.BrowseResultMaskAll),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("browse %s: %w", node, err)
	}
	if len(response.Results) == 0 {
		return nil, nil
	}

	result := response.Results[0]
	if result.StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("browse %s: %s", node, StatusName(result.StatusCode))
	}

	// References are collected through a seen-set rather than appended blindly.
	// A server is supposed to hand back only the remainder with each
	// continuation point, but implementations exist whose pages overlap, and a
	// node listed twice reads as two different nodes. Stopping when a page adds
	// nothing new also bounds a server whose continuation point never advances.
	seen := map[string]bool{}
	references := dedupe(nil, result.References, seen)

	for continuation := result.ContinuationPoint; len(continuation) > 0; {
		next, err := w.client.BrowseNext(ctx, &ua.BrowseNextRequest{ContinuationPoints: [][]byte{continuation}})
		if err != nil {
			return nil, fmt.Errorf("browse next %s: %w", node, err)
		}
		if len(next.Results) == 0 {
			break
		}
		if next.Results[0].StatusCode != ua.StatusOK {
			return nil, fmt.Errorf("browse next %s: %s", node, StatusName(next.Results[0].StatusCode))
		}

		before := len(references)
		references = dedupe(references, next.Results[0].References, seen)
		if len(references) == before {
			break
		}
		continuation = next.Results[0].ContinuationPoint
	}

	return references, nil
}

// dedupe appends the references not already in seen. Two references to the same
// node through different reference types are distinct and both are kept; an
// exact repeat is not.
func dedupe(into []*ua.ReferenceDescription, from []*ua.ReferenceDescription, seen map[string]bool) []*ua.ReferenceDescription {
	for _, reference := range from {
		if reference == nil || reference.NodeID == nil || reference.NodeID.NodeID == nil {
			continue
		}

		key := fmt.Sprintf("%s|%v|%s", reference.NodeID.NodeID, reference.IsForward, reference.ReferenceTypeID)
		if seen[key] {
			continue
		}
		seen[key] = true
		into = append(into, reference)
	}
	return into
}

// readValues reads the value, data type and access level of the variables at the
// given positions, in one Read call for the whole level.
func (w *walker) readValues(ctx context.Context, nodes []cligen.Node, positions []int) error {
	attributes := []ua.AttributeID{ua.AttributeIDValue, ua.AttributeIDDataType, ua.AttributeIDAccessLevel}

	toRead := make([]*ua.ReadValueID, 0, len(positions)*len(attributes))
	readable := make([]int, 0, len(positions))
	for _, position := range positions {
		node, err := ParseNodeID(nodes[position].NodeID)
		if err != nil {
			continue
		}
		readable = append(readable, position)
		for _, attribute := range attributes {
			toRead = append(toRead, &ua.ReadValueID{NodeID: node, AttributeID: attribute})
		}
	}
	if len(toRead) == 0 {
		return nil
	}

	response, err := w.client.Read(ctx, &ua.ReadRequest{
		NodesToRead:        toRead,
		TimestampsToReturn: ua.TimestampsToReturnNeither,
	})
	if err != nil {
		return fmt.Errorf("read values: %w", err)
	}

	for index, position := range readable {
		base := index * len(attributes)
		if base+len(attributes) > len(response.Results) {
			break
		}

		if value := response.Results[base]; value != nil {
			status := StatusName(value.Status)
			nodes[position].ValueStatus = &status
			if value.Status == ua.StatusOK {
				nodes[position].Value = JSON(value.Value)
			}
		}
		if dataType := response.Results[base+1]; dataType != nil && dataType.Status == ua.StatusOK {
			name := dataTypeText(dataType.Value)
			nodes[position].DataType = &name
		}
		if access := response.Results[base+2]; access != nil && access.Status == ua.StatusOK {
			level := AccessLevelText(ua.AccessLevelType(access.Value.Int()))
			nodes[position].AccessLevel = &level
		}
	}

	return nil
}

// describe turns a reference description into a reported node.
func describe(reference *ua.ReferenceDescription, path string, depth int) (cligen.Node, bool) {
	if reference == nil || reference.NodeID == nil || reference.NodeID.NodeID == nil {
		return cligen.Node{}, false
	}

	target := reference.NodeID.NodeID
	name := qualifiedText(reference.BrowseName)

	node := cligen.Node{
		NodeID:     target.String(),
		BrowseName: name,
		NodeClass:  NodeClassName(reference.NodeClass),
		Namespace:  new(int(target.Namespace())),
		Depth:      new(depth),
		IsForward:  new(reference.IsForward),
	}

	fullPath := name
	if path != "" {
		fullPath = path + "/" + name
	}
	node.Path = &fullPath

	if reference.DisplayName != nil && reference.DisplayName.Text != "" && reference.DisplayName.Text != name {
		node.DisplayName = new(reference.DisplayName.Text)
	}
	if referenceName := referenceText(reference.ReferenceTypeID); referenceName != "" {
		node.ReferenceType = &referenceName
	}
	if reference.TypeDefinition != nil && reference.TypeDefinition.NodeID != nil {
		definition := nodeIDText(reference.TypeDefinition.NodeID)
		node.TypeDefinition = &definition
	}

	return node, true
}

// nodePath is the browse path a node's children hang off.
func nodePath(node cligen.Node) string {
	if node.Path != nil {
		return *node.Path
	}
	return node.BrowseName
}

// NodeClassFromName is the inverse of NodeClassName, for filtering a described
// node by class without re-reading the attribute.
//
// The two directions of the gopcua mapping are not symmetric: the stringer emits
// the constant's name ("NodeClassVariable") while the parser expects the bare
// one ("Variable"), so the trimmed form is what crosses between them.
func NodeClassFromName(name string) ua.NodeClass {
	return ua.NodeClassFromString(strings.TrimPrefix(name, "NodeClass"))
}

// qualifiedText renders a browse name, keeping the namespace prefix when it is
// not the standard namespace, since that prefix is part of the name a browse
// path has to spell.
func qualifiedText(name *ua.QualifiedName) string {
	if name == nil {
		return ""
	}
	if name.NamespaceIndex == 0 {
		return name.Name
	}
	return fmt.Sprintf("%d:%s", name.NamespaceIndex, name.Name)
}

// referenceText names a reference type, preferring the specification's name.
func referenceText(reference *ua.NodeID) string {
	if reference == nil {
		return ""
	}
	if name := StandardName(reference); name != "" {
		return name
	}
	return reference.String()
}

// dataTypeText names the data type a DataType attribute points at.
func dataTypeText(variant *ua.Variant) string {
	if variant == nil {
		return ""
	}
	node := variant.NodeID()
	if node == nil {
		return Text(variant)
	}
	if name := StandardName(node); name != "" {
		return name
	}
	return node.String()
}

// MatchName builds the name filter shared by browse and find: a
// case-insensitive substring test, or a regular expression, over the chosen part
// of a node.
func MatchName(pattern, field string, isRegex bool) (func(cligen.Node) bool, error) {
	if pattern == "" {
		return nil, nil
	}

	candidates := func(node cligen.Node) []string {
		display := ""
		if node.DisplayName != nil {
			display = *node.DisplayName
		}
		switch field {
		case "browse-name":
			return []string{node.BrowseName}
		case "display-name":
			return []string{display}
		case "node-id":
			return []string{node.NodeID}
		default:
			return []string{node.BrowseName, display, node.NodeID}
		}
	}

	if isRegex {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern: %w", err)
		}
		return func(node cligen.Node) bool {
			for _, candidate := range candidates(node) {
				if expression.MatchString(candidate) {
					return true
				}
			}
			return false
		}, nil
	}

	needle := strings.ToLower(pattern)
	return func(node cligen.Node) bool {
		for _, candidate := range candidates(node) {
			if strings.Contains(strings.ToLower(candidate), needle) {
				return true
			}
		}
		return false
	}, nil
}

// MatchNamespaces builds a filter that keeps only nodes in the given namespace
// indexes. An empty list keeps everything.
func MatchNamespaces(namespaces []string) (func(cligen.Node) bool, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	wanted := map[int]bool{}
	for _, text := range namespaces {
		index, err := parseIndex(text)
		if err != nil {
			return nil, err
		}
		wanted[index] = true
	}

	return func(node cligen.Node) bool {
		if node.Namespace == nil {
			return false
		}
		return wanted[*node.Namespace]
	}, nil
}

// AllOf combines filters, keeping only nodes every non-nil filter accepts.
func AllOf(filters ...func(cligen.Node) bool) func(cligen.Node) bool {
	var active []func(cligen.Node) bool
	for _, filter := range filters {
		if filter != nil {
			active = append(active, filter)
		}
	}
	if len(active) == 0 {
		return nil
	}

	return func(node cligen.Node) bool {
		for _, filter := range active {
			if !filter(node) {
				return false
			}
		}
		return true
	}
}
