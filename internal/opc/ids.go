// Package opc holds the OPC UA domain logic that sits between the gopcua
// client and the command handlers: parsing the identifiers a user types,
// naming the numeric constants the protocol uses, converting variants to and
// from text and JSON, and the browse, read, type and event operations the
// commands are built from.
//
// Results are returned as the generated schema types from the OpenCLI
// specification, which is the single declared shape of this tool's output.
package opc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
)

// WellKnownNodes are the entry points into any server's address space, accepted
// wherever a node id is expected so that `opcua browse Server` works without
// looking up i=2253.
var WellKnownNodes = map[string]uint32{
	"root":                 id.RootFolder,
	"objects":              id.ObjectsFolder,
	"types":                id.TypesFolder,
	"views":                id.ViewsFolder,
	"objecttypes":          id.ObjectTypesFolder,
	"variabletypes":        id.VariableTypesFolder,
	"datatypes":            id.DataTypesFolder,
	"referencetypes":       id.ReferenceTypesFolder,
	"eventtypes":           id.EventTypesFolder,
	"server":               id.Server,
	"serverstatus":         id.Server_ServerStatus,
	"serverdiag":           id.Server_ServerDiagnostics,
	"servercaps":           id.Server_ServerCapabilities,
	"namespacearray":       id.Server_NamespaceArray,
	"basedatatype":         id.BaseDataType,
	"baseobjecttype":       id.BaseObjectType,
	"baseeventtype":        id.BaseEventType,
	"basedatavariabletype": id.BaseDataVariableType,
}

// ParseNodeID accepts the standard textual node id forms (i=, s=, g=, b=, with
// an optional ns= prefix) plus two conveniences: a bare number is a numeric id
// in namespace 0, and a well-known name such as "Objects" or "Server" resolves
// to its standard identifier.
func ParseNodeID(text string) (*ua.NodeID, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("empty node id")
	}

	if numeric, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		return ua.NewNumericNodeID(0, uint32(numeric)), nil
	}
	if known, ok := WellKnownNodes[strings.ToLower(trimmed)]; ok {
		return ua.NewNumericNodeID(0, known), nil
	}

	parsed, err := ua.ParseNodeID(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid node id %q: %w", text, err)
	}
	return parsed, nil
}

// ParseNodeIDs parses a list, reporting the first failure.
func ParseNodeIDs(texts []string) ([]*ua.NodeID, error) {
	ids := make([]*ua.NodeID, 0, len(texts))
	for _, text := range texts {
		parsed, err := ParseNodeID(text)
		if err != nil {
			return nil, err
		}
		ids = append(ids, parsed)
	}
	return ids, nil
}

// attributes is every attribute defined by the specification, in
// specification order: one table serving name lookup, id lookup, and the
// attributes command's "read everything" request.
var attributes = []struct {
	Name string
	ID   ua.AttributeID
}{
	{"NodeId", ua.AttributeIDNodeID},
	{"NodeClass", ua.AttributeIDNodeClass},
	{"BrowseName", ua.AttributeIDBrowseName},
	{"DisplayName", ua.AttributeIDDisplayName},
	{"Description", ua.AttributeIDDescription},
	{"WriteMask", ua.AttributeIDWriteMask},
	{"UserWriteMask", ua.AttributeIDUserWriteMask},
	{"IsAbstract", ua.AttributeIDIsAbstract},
	{"Symmetric", ua.AttributeIDSymmetric},
	{"InverseName", ua.AttributeIDInverseName},
	{"ContainsNoLoops", ua.AttributeIDContainsNoLoops},
	{"EventNotifier", ua.AttributeIDEventNotifier},
	{"Value", ua.AttributeIDValue},
	{"DataType", ua.AttributeIDDataType},
	{"ValueRank", ua.AttributeIDValueRank},
	{"ArrayDimensions", ua.AttributeIDArrayDimensions},
	{"AccessLevel", ua.AttributeIDAccessLevel},
	{"UserAccessLevel", ua.AttributeIDUserAccessLevel},
	{"MinimumSamplingInterval", ua.AttributeIDMinimumSamplingInterval},
	{"Historizing", ua.AttributeIDHistorizing},
	{"Executable", ua.AttributeIDExecutable},
	{"UserExecutable", ua.AttributeIDUserExecutable},
	{"DataTypeDefinition", ua.AttributeIDDataTypeDefinition},
	{"RolePermissions", ua.AttributeIDRolePermissions},
	{"UserRolePermissions", ua.AttributeIDUserRolePermissions},
	{"AccessRestrictions", ua.AttributeIDAccessRestrictions},
	{"AccessLevelEx", ua.AttributeIDAccessLevelEx},
}

// AllAttributes is every attribute id in specification order.
func AllAttributes() []ua.AttributeID {
	ids := make([]ua.AttributeID, 0, len(attributes))
	for _, attribute := range attributes {
		ids = append(ids, attribute.ID)
	}
	return ids
}

// AttributeNames lists every attribute name in specification order, for help
// text and shell completion.
func AttributeNames() []string {
	names := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		names = append(names, attribute.Name)
	}
	return names
}

// ParseAttribute accepts an attribute name in any case, with or without
// separators ("access-level", "AccessLevel", "accesslevel"), or its numeric id.
func ParseAttribute(text string) (ua.AttributeID, error) {
	trimmed := strings.TrimSpace(text)
	if numeric, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		attribute := ua.AttributeID(numeric)
		if AttributeName(attribute) == "" {
			return 0, fmt.Errorf("unknown attribute id %d", numeric)
		}
		return attribute, nil
	}

	key := foldName(trimmed)
	for _, attribute := range attributes {
		if foldName(attribute.Name) == key {
			return attribute.ID, nil
		}
	}
	return 0, fmt.Errorf("unknown attribute %q (known: %s)", text, strings.Join(AttributeNames(), ", "))
}

// AttributeName is the specification's name for an attribute id, or "" when the
// id is not one.
func AttributeName(id ua.AttributeID) string {
	for _, attribute := range attributes {
		if attribute.ID == id {
			return attribute.Name
		}
	}
	return ""
}

// referenceTypes are the reference types worth naming on a command line, keyed
// by their name folded to lower case. A reference type not listed here can
// still be given as a node id.
var referenceTypes = []uint32{
	id.References,
	id.HierarchicalReferences,
	id.NonHierarchicalReferences,
	id.HasChild,
	id.Aggregates,
	id.HasComponent,
	id.HasProperty,
	id.HasSubtype,
	id.Organizes,
	id.HasModellingRule,
	id.HasTypeDefinition,
	id.HasEventSource,
	id.HasNotifier,
	id.HasEncoding,
	id.HasDescription,
	id.GeneratesEvent,
	id.AlwaysGeneratesEvent,
	id.HasCause,
	id.HasEffect,
	id.HasTrueSubState,
	id.HasFalseSubState,
	id.HasCondition,
	id.HasOrderedComponent,
	id.HasInterface,
	id.HasAddIn,
	id.HasDictionaryEntry,
	id.HasStructuredComponent,
	id.HasGuard,
}

// ReferenceTypeNames lists the nameable reference types alphabetically, for
// help text and shell completion.
func ReferenceTypeNames() []string {
	names := make([]string, 0, len(referenceTypes)+1)
	names = append(names, "all")
	for _, numeric := range referenceTypes {
		names = append(names, id.Name(numeric))
	}
	sort.Strings(names)
	return names
}

// ParseReferenceType accepts a reference type name, a node id, or "all" for
// every reference (the References supertype, which every reference derives
// from).
func ParseReferenceType(text string) (*ua.NodeID, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.EqualFold(trimmed, "all") {
		return ua.NewNumericNodeID(0, id.References), nil
	}

	key := foldName(trimmed)
	for _, numeric := range referenceTypes {
		if foldName(id.Name(numeric)) == key {
			return ua.NewNumericNodeID(0, numeric), nil
		}
	}
	return ParseNodeID(trimmed)
}

// StandardName is the specification's name for a node in namespace 0, or "" for
// a node the specification does not define. It turns an opaque `i=2258` in
// output into `CurrentTime`.
func StandardName(node *ua.NodeID) string {
	if node == nil || node.Namespace() != 0 {
		return ""
	}
	switch node.Type() {
	case ua.NodeIDTypeNumeric, ua.NodeIDTypeTwoByte, ua.NodeIDTypeFourByte:
		return id.Name(node.IntID())
	default:
		return ""
	}
}

// foldName reduces an identifier to a case- and separator-insensitive key, so
// "has-component", "HasComponent" and "hascomponent" all match.
func foldName(text string) string {
	return strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(text)))
}

// parseIndex reads a namespace index, which a user may write with surrounding
// space when it came from a comma-separated list.
func parseIndex(text string) (int, error) {
	index, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || index < 0 {
		return 0, fmt.Errorf("invalid namespace index %q", text)
	}
	return index, nil
}

// nodeClassNames maps a node class name to its mask bit.
var nodeClassNames = map[string]ua.NodeClass{
	"object":        ua.NodeClassObject,
	"variable":      ua.NodeClassVariable,
	"method":        ua.NodeClassMethod,
	"objecttype":    ua.NodeClassObjectType,
	"variabletype":  ua.NodeClassVariableType,
	"referencetype": ua.NodeClassReferenceType,
	"datatype":      ua.NodeClassDataType,
	"view":          ua.NodeClassView,
}

// NodeClassNames lists the node class names accepted by --class.
func NodeClassNames() []string {
	names := make([]string, 0, len(nodeClassNames))
	for name := range nodeClassNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ParseNodeClassMask turns a list of class names into a browse node class mask.
// An empty list means every class.
func ParseNodeClassMask(classes []string) (ua.NodeClass, error) {
	if len(classes) == 0 {
		return ua.NodeClassAll, nil
	}

	var mask ua.NodeClass
	for _, class := range classes {
		key := foldName(class)
		if key == "" {
			continue
		}
		if key == "all" {
			return ua.NodeClassAll, nil
		}
		bit, ok := nodeClassNames[key]
		if !ok {
			return 0, fmt.Errorf("unknown node class %q (known: %s)", class, strings.Join(NodeClassNames(), ", "))
		}
		mask |= bit
	}
	if mask == 0 {
		return ua.NodeClassAll, nil
	}
	return mask, nil
}

// NodeClassName is the readable name of a node class.
func NodeClassName(class ua.NodeClass) string {
	name := class.String()
	// The generated stringer spells the constant, e.g. "NodeClassVariable".
	return strings.TrimPrefix(name, "NodeClass")
}

// BrowseDirection maps a direction name to the protocol enumeration.
func BrowseDirection(name string) ua.BrowseDirection {
	switch strings.ToLower(name) {
	case "inverse":
		return ua.BrowseDirectionInverse
	case "both":
		return ua.BrowseDirectionBoth
	default:
		return ua.BrowseDirectionForward
	}
}

// TimestampsToReturn maps a timestamp selection name to the protocol
// enumeration.
func TimestampsToReturn(name string) ua.TimestampsToReturn {
	switch strings.ToLower(name) {
	case "source":
		return ua.TimestampsToReturnSource
	case "server":
		return ua.TimestampsToReturnServer
	case "neither":
		return ua.TimestampsToReturnNeither
	default:
		return ua.TimestampsToReturnBoth
	}
}
