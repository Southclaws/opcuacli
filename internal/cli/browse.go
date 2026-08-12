package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gopcua/opcua/ua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// browse walks the address space from a node and reports what it finds.
func browse(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.BrowseParams) (cligen.NodeList, error) {
	root, err := node(p.Node)
	if err != nil {
		return nil, err
	}
	referenceType, err := opc.ParseReferenceType(p.RefType)
	if err != nil {
		return nil, err
	}
	classMask, err := opc.ParseNodeClassMask(p.Class)
	if err != nil {
		return nil, err
	}
	match, err := opc.MatchName(p.Filter, "any", p.Regex)
	if err != nil {
		return nil, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	format := string(p.Format)
	options := opc.BrowseOptions{
		Root:            root,
		Depth:           p.Depth,
		Direction:       opc.BrowseDirection(string(p.Direction)),
		ReferenceType:   referenceType,
		IncludeSubtypes: !p.NoSubtypes,
		ClassMask:       classMask,
		Match:           match,
		WithValues:      p.Values,
		Limit:           p.Limit,
		Nested:          format == "tree",
	}
	if format == "jsonl" {
		options.Emit = jsonlEmitter(session.Out)
	}

	nodes, err := opc.Browse(ctx, session.Client, options)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	if machineReadable(format) {
		return nodes, nil
	}

	switch format {
	case "jsonl":
		// Already streamed as the walk proceeded.
	case "tree":
		label := fmt.Sprintf("%s %s", root, opc.StandardName(root))
		session.Out.Tree(session.Out.T.Styles.Title.Render(strings.TrimSpace(label)), nodeTree(session.Out, nodes, p.Values))
	case "plain":
		session.Out.Plain(nodeRows(nodes, p.Values, false))
	default:
		renderNodeTable(session.Out, nodes, p.Values)
	}

	return nodes, nil
}

// find crawls the address space looking for nodes whose name matches.
func find(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.FindParams) (cligen.NodeList, error) {
	root, err := node(p.Root)
	if err != nil {
		return nil, err
	}
	classMask, err := opc.ParseNodeClassMask(p.Class)
	if err != nil {
		return nil, err
	}
	byName, err := opc.MatchName(p.Pattern, string(p.Match), p.Regex)
	if err != nil {
		return nil, err
	}
	byNamespace, err := opc.MatchNamespaces(p.Namespace)
	if err != nil {
		return nil, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	format := string(p.Format)
	options := opc.BrowseOptions{
		Root:            root,
		Depth:           p.MaxDepth,
		Direction:       ua.BrowseDirectionForward,
		IncludeSubtypes: true,
		ClassMask:       classMask,
		Match:           opc.AllOf(byName, byNamespace),
		WithValues:      p.Values,
		Limit:           p.Limit,
	}
	if format == "jsonl" {
		options.Emit = jsonlEmitter(session.Out)
	}

	nodes, err := opc.Browse(ctx, session.Client, options)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	if machineReadable(format) {
		return nodes, nil
	}

	switch format {
	case "jsonl":
		// Already streamed as the crawl proceeded.
	case "plain":
		session.Out.Plain(nodeRows(nodes, p.Values, true))
	default:
		if len(nodes) == 0 {
			session.Out.Notef("no node matching %q under %s", p.Pattern, root)
			return nodes, nil
		}
		session.Out.Title(fmt.Sprintf("%q", p.Pattern), fmt.Sprintf("%d match(es) under %s", len(nodes), root))
		renderNodeTable(session.Out, nodes, p.Values)
		if p.Limit > 0 && len(nodes) >= p.Limit {
			session.Err.Warnf("stopped at the --limit of %d; there may be more", p.Limit)
		}
	}

	return nodes, nil
}

// resolve translates browse paths into node ids, letting the server do the walk.
func resolve(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ResolveParams) (cligen.ResolvedPathList, error) {
	root, err := node(p.Root)
	if err != nil {
		return nil, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	resolved, err := opc.Resolve(ctx, session.Client, root, p.Path, uint16(p.Namespace))
	if err != nil {
		// Translating a browse path is an optional service. When a server has
		// not implemented it, a client-side crawl is the way to get the same
		// answer, so say so rather than leaving the user with a status code.
		var fault *opc.ServiceError
		if errors.As(err, &fault) && fault.Unsupported() {
			return nil, fmt.Errorf("%w: this server does not implement TranslateBrowsePathsToNodeIds; "+
				"find the node by name instead, e.g. opcua find %s", ErrService, lastElement(p.Path))
		}
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return resolved, nil
	}

	failed := 0
	rows := make([][]string, 0, len(resolved))
	for _, entry := range resolved {
		if entry.NodeID == nil {
			failed++
		}
		rows = append(rows, []string{entry.Path, derefOr(entry.NodeID, "-"), entry.Status})
	}

	switch format {
	case "table":
		session.Out.Table([]string{"PATH", "NODE ID", "STATUS"}, rows)
	default:
		// Plain output is one node id per line so a resolved path pipes straight
		// into read, write or monitor.
		for _, entry := range resolved {
			if entry.NodeID == nil {
				session.Err.Warnf("%s: %s", entry.Path, entry.Status)
				continue
			}
			fmt.Fprintln(session.Out.Raw(), *entry.NodeID)
		}
	}

	if failed == len(resolved) && failed > 0 {
		return resolved, fmt.Errorf("%w: no path resolved", ErrNotFound)
	}
	return resolved, nil
}

// references lists one node's references, reference type and all.
func references(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ReferencesParams) (cligen.ReferenceList, error) {
	target, err := node(p.Node)
	if err != nil {
		return nil, err
	}
	referenceType, err := opc.ParseReferenceType(p.RefType)
	if err != nil {
		return nil, err
	}
	classMask, err := opc.ParseNodeClassMask(p.Class)
	if err != nil {
		return nil, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	found, err := opc.References(ctx, session.Client, opc.ReferenceOptions{
		Node:            target,
		Direction:       opc.BrowseDirection(string(p.Direction)),
		ReferenceType:   referenceType,
		IncludeSubtypes: !p.NoSubtypes,
		ClassMask:       classMask,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return found, nil
	}

	rows := make([][]string, 0, len(found))
	for _, reference := range found {
		rows = append(rows, []string{
			direction(session.Out, reference.IsForward),
			reference.ReferenceType,
			reference.BrowseName,
			reference.NodeClass,
			reference.NodeID,
		})
	}

	if format == "plain" {
		session.Out.Plain(rows)
		return found, nil
	}

	session.Out.Title(target.String(), fmt.Sprintf("%d reference(s)", len(found)))
	session.Out.Table([]string{"DIR", "REFERENCE", "NAME", "CLASS", "NODE ID"}, rows)
	return found, nil
}

// direction renders which way a reference points, since an inverse reference
// means the opposite relationship and is easy to misread in a list.
func direction(p *render.Printer, isForward bool) string {
	if isForward {
		return p.T.Symbols.Arrow
	}
	return p.T.Symbols.Inverse
}

// jsonlEmitter streams one node per line as it is found, so a long walk produces
// output immediately rather than after it finishes.
func jsonlEmitter(p *render.Printer) func(cligen.Node) error {
	encoder := json.NewEncoder(p.Raw())
	return func(found cligen.Node) error {
		return encoder.Encode(found)
	}
}

// renderNodeTable renders a flat node list, adding a value column only when
// values were read.
//
// Cells are truncated rather than wrapped: a browse path is long and its tail is
// the informative part, and a wrapped path turns one node into three lines,
// which makes a listing of a real plant unreadable.
func renderNodeTable(p *render.Printer, nodes cligen.NodeList, withValues bool) {
	headers := []string{"CLASS", "NAME", "NODE ID", "PATH"}
	if withValues {
		headers = []string{"CLASS", "NAME", "NODE ID", "TYPE", "VALUE"}
	}

	// Budget: the class column is fixed, and what is left goes to the three
	// variable-width columns.
	budget := max((p.Width-12)/3, 16)

	rows := make([][]string, 0, len(nodes))
	for _, found := range nodes {
		name := render.Truncate(found.BrowseName, budget)
		nodeID := render.Truncate(found.NodeID, budget)

		if withValues {
			rows = append(rows, []string{
				classLabel(p, found.NodeClass),
				name,
				nodeID,
				render.Truncate(derefOr(found.DataType, ""), 20),
				valueCell(p, found),
			})
			continue
		}
		rows = append(rows, []string{
			classLabel(p, found.NodeClass),
			name,
			nodeID,
			p.T.Styles.Path.Render(tailTruncate(derefOr(found.Path, ""), budget)),
		})
	}

	p.Table(headers, rows)
}

// lastElement is the final element of the first browse path given, which is the
// name a search would look for.
func lastElement(paths []string) string {
	if len(paths) == 0 {
		return "NAME"
	}
	elements := strings.Split(strings.Trim(paths[0], "/"), "/")
	return elements[len(elements)-1]
}

// tailTruncate keeps the end of a path rather than its start, since a deep path
// shares its prefix with every sibling and only differs at the tail. Width is
// measured in display cells, as the terminal counts them.
func tailTruncate(path string, width int) string {
	if width <= 1 || lipgloss.Width(path) <= width {
		return path
	}

	runes := []rune(path)
	for start := 0; start < len(runes); start++ {
		candidate := "…" + string(runes[start:])
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
	}
	return "…"
}

// nodeRows renders a flat node list for plain output. The node id comes first so
// that piping into another command works by taking the first field.
func nodeRows(nodes cligen.NodeList, withValues, withPath bool) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, found := range nodes {
		row := []string{found.NodeID, found.NodeClass, found.BrowseName}
		if withPath {
			row = append(row, derefOr(found.Path, ""))
		}
		if withValues {
			row = append(row, valueText(found))
		}
		rows = append(rows, row)
	}
	return rows
}

// nodeTree builds the renderable tree for a nested walk.
func nodeTree(p *render.Printer, nodes cligen.NodeList, withValues bool) []render.TreeNode {
	branches := make([]render.TreeNode, 0, len(nodes))
	for _, found := range nodes {
		branches = append(branches, render.TreeNode{
			Label:    treeLabel(p, found, withValues),
			Children: nodeTree(p, found.Children, withValues),
		})
	}
	return branches
}

// treeLabel is one line of the tree: the name, its class, and its value when one
// was read, styled so the eye can pick out the parts.
func treeLabel(p *render.Printer, found cligen.Node, withValues bool) string {
	styles := p.T.Styles

	label := styles.Value.Render(found.BrowseName)
	label += " " + styles.Faint.Render(found.NodeID)
	if found.NodeClass != "Object" {
		label += " " + classLabel(p, found.NodeClass)
	}

	if withValues && found.Value != nil {
		label += "  " + styles.Accent.Render("= "+render.Truncate(valueText(found), 48))
	} else if withValues && found.ValueStatus != nil && !strings.HasPrefix(*found.ValueStatus, "Good") {
		label += "  " + styles.Bad.Render(*found.ValueStatus)
	}

	return label
}

// classLabel abbreviates a node class to the four characters that distinguish
// them, so the column stays narrow in a wide listing.
func classLabel(p *render.Printer, class string) string {
	short := map[string]string{
		"Object":        "obj",
		"Variable":      "var",
		"Method":        "fn",
		"ObjectType":    "otyp",
		"VariableType":  "vtyp",
		"ReferenceType": "rtyp",
		"DataType":      "dtyp",
		"View":          "view",
	}[class]
	if short == "" {
		short = strings.ToLower(class)
	}

	switch class {
	case "Variable":
		return p.T.Styles.Subtitle.Render(short)
	case "Method":
		return p.T.Styles.Class.Render(short)
	case "Object":
		return p.T.Styles.Dim.Render(short)
	default:
		return p.T.Styles.Type.Render(short)
	}
}

// valueCell renders a node's value for a table cell, colouring a bad status so a
// failed read is not mistaken for an empty value.
func valueCell(p *render.Printer, found cligen.Node) string {
	if found.Value == nil {
		if found.ValueStatus != nil && !strings.HasPrefix(*found.ValueStatus, "Good") {
			return p.T.Styles.Bad.Render(*found.ValueStatus)
		}
		return ""
	}
	return render.Truncate(valueText(found), 60)
}

// valueText renders a node's value as text, from the JSON-shaped value the walk
// collected.
func valueText(found cligen.Node) string {
	switch value := found.Value.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, fmt.Sprint(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprint(value)
	}
}
