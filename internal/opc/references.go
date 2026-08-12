package opc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// ReferenceOptions controls a listing of one node's references.
type ReferenceOptions struct {
	Node            *ua.NodeID
	Direction       ua.BrowseDirection
	ReferenceType   *ua.NodeID
	IncludeSubtypes bool
	ClassMask       ua.NodeClass
}

// References lists every reference of a node, reference type and all.
//
// Where a browse presents the address space as a hierarchy, this presents the
// raw graph: type definitions, modelling rules, event sources and the other
// non-hierarchical links that a hierarchical walk hides.
func References(ctx context.Context, client Reader, options ReferenceOptions) ([]cligen.Reference, error) {
	if options.ReferenceType == nil {
		reference, err := ParseReferenceType("all")
		if err != nil {
			return nil, err
		}
		options.ReferenceType = reference
	}
	if options.ClassMask == 0 {
		options.ClassMask = ua.NodeClassAll
	}

	walk := &walker{
		client: client,
		options: BrowseOptions{
			Direction:       options.Direction,
			ReferenceType:   options.ReferenceType,
			IncludeSubtypes: options.IncludeSubtypes,
		},
		visited: map[string]bool{},
	}

	descriptions, err := walk.references(ctx, options.Node)
	if err != nil {
		return nil, err
	}

	references := make([]cligen.Reference, 0, len(descriptions))
	for _, description := range descriptions {
		if description == nil || description.NodeID == nil || description.NodeID.NodeID == nil {
			continue
		}
		if options.ClassMask != ua.NodeClassAll && description.NodeClass&options.ClassMask == 0 {
			continue
		}

		reference := cligen.Reference{
			ReferenceType: referenceText(description.ReferenceTypeID),
			IsForward:     description.IsForward,
			NodeID:        description.NodeID.NodeID.String(),
			BrowseName:    qualifiedText(description.BrowseName),
			NodeClass:     NodeClassName(description.NodeClass),
		}
		if description.ReferenceTypeID != nil {
			reference.ReferenceTypeID = new(description.ReferenceTypeID.String())
		}
		if description.DisplayName != nil && description.DisplayName.Text != "" {
			reference.DisplayName = new(description.DisplayName.Text)
		}
		if description.TypeDefinition != nil && description.TypeDefinition.NodeID != nil {
			reference.TypeDefinition = new(nodeIDText(description.TypeDefinition.NodeID))
		}

		references = append(references, reference)
	}

	return references, nil
}

// Sender issues a service call the client has no dedicated method for. The
// TranslateBrowsePathsToNodeIds service is one of those: gopcua exposes it only
// for a single path at a time, and resolving several in one round trip is the
// whole point of the service.
type Sender interface {
	Send(context.Context, ua.Request, func(ua.Response) error) error
}

// Resolve translates browse paths into node ids, in one service call.
//
// The server does the walking, which is both faster and more correct than
// crawling: it knows which references are hierarchical and which browse names
// are unique.
//
// Path elements are separated by '/' and may carry an explicit namespace as
// "2:Name"; elements without one use defaultNamespace. Each result carries its
// own status code, so one bad path does not spoil the rest.
func Resolve(ctx context.Context, client Sender, root *ua.NodeID, paths []string, defaultNamespace uint16) ([]cligen.ResolvedPath, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	requests := make([]*ua.BrowsePath, 0, len(paths))
	for _, path := range paths {
		elements, err := browsePathElements(path, defaultNamespace)
		if err != nil {
			return nil, err
		}
		requests = append(requests, &ua.BrowsePath{
			StartingNode: root,
			RelativePath: &ua.RelativePath{Elements: elements},
		})
	}

	var response *ua.TranslateBrowsePathsToNodeIDsResponse
	err := client.Send(ctx, &ua.TranslateBrowsePathsToNodeIDsRequest{BrowsePaths: requests},
		func(received ua.Response) error {
			typed, ok := received.(*ua.TranslateBrowsePathsToNodeIDsResponse)
			if !ok {
				return fmt.Errorf("unexpected response %T to translate browse paths", received)
			}
			response = typed
			return nil
		})
	if err != nil {
		return nil, serviceError(err)
	}
	if err := serviceResult(response.ResponseHeader); err != nil {
		return nil, err
	}

	resolved := make([]cligen.ResolvedPath, 0, len(paths))
	for index, path := range paths {
		entry := cligen.ResolvedPath{Path: path, Status: StatusName(ua.StatusBadUnexpectedError)}
		if index < len(response.Results) && response.Results[index] != nil {
			result := response.Results[index]
			entry.Status = StatusName(result.StatusCode)
			for _, target := range result.Targets {
				if target == nil || target.TargetID == nil || target.TargetID.NodeID == nil {
					continue
				}
				entry.NodeID = new(target.TargetID.NodeID.String())
				break
			}
		}
		resolved = append(resolved, entry)
	}

	return resolved, nil
}

// browsePathElements parses a slash-separated browse path into relative path
// elements. Every element follows any hierarchical reference, which is what a
// path written by hand means.
func browsePathElements(path string, defaultNamespace uint16) ([]*ua.RelativePathElement, error) {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("empty browse path")
	}

	parts := strings.Split(trimmed, "/")
	elements := make([]*ua.RelativePathElement, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("browse path %q has an empty element", path)
		}

		namespace := defaultNamespace
		name := part
		if index, rest, found := strings.Cut(part, ":"); found {
			if parsed, err := strconv.ParseUint(index, 10, 16); err == nil {
				namespace = uint16(parsed)
				name = rest
			}
		}

		elements = append(elements, &ua.RelativePathElement{
			ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
			IncludeSubtypes: true,
			TargetName:      &ua.QualifiedName{NamespaceIndex: namespace, Name: name},
		})
	}

	return elements, nil
}
