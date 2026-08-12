package opc

import (
	"context"
	"fmt"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// TypeOf reports the declared type of a variable: its DataType attribute
// resolved to a name, the built-in type that data type ultimately derives from,
// and the shape its ValueRank allows.
//
// The built-in type is the useful part for writing: a server rejects a write
// whose variant type does not match, and a custom data type such as an
// enumeration is written as the Int32 it derives from.
func TypeOf(ctx context.Context, client Reader, node *ua.NodeID) (cligen.TypeOf, error) {
	// DataType, ValueRank and ArrayDimensions together describe the shape of a
	// value, so they are read in one call.
	response, err := client.Read(ctx, &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnNeither,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: node, AttributeID: ua.AttributeIDDataType},
			{NodeID: node, AttributeID: ua.AttributeIDValueRank},
			{NodeID: node, AttributeID: ua.AttributeIDArrayDimensions},
		},
	})
	if err != nil {
		return cligen.TypeOf{}, err
	}
	if err := serviceResult(response.ResponseHeader); err != nil {
		return cligen.TypeOf{}, err
	}
	if len(response.Results) < 3 {
		return cligen.TypeOf{}, fmt.Errorf("read %s: server returned %d of 3 attributes", node, len(response.Results))
	}

	declared := response.Results[0]
	if declared.Status != ua.StatusOK {
		return cligen.TypeOf{}, fmt.Errorf("read %s DataType: %s", node, StatusName(declared.Status))
	}
	dataType := declared.Value.NodeID()
	if dataType == nil {
		return cligen.TypeOf{}, fmt.Errorf("node %s has no DataType attribute; only a variable has a data type", node)
	}

	result := cligen.TypeOf{
		NodeID:     node.String(),
		DataType:   typeNodeName(dataType),
		DataTypeID: dataType.String(),
	}

	if rank := response.Results[1]; rank.Status == ua.StatusOK {
		value := int(rank.Value.Int())
		result.ValueRank = &value
		text := ValueRankText(int32(value))
		result.Rank = &text
	}
	if arrayDimensions := response.Results[2]; arrayDimensions.Status == ua.StatusOK {
		result.ArrayDimensions = dimensions(arrayDimensions.Value)
	}

	chain, err := SuperTypes(ctx, client, dataType)
	if err == nil {
		for _, ancestor := range chain {
			result.SuperTypes = append(result.SuperTypes, typeNodeName(ancestor))
		}
	}

	if builtin, ok := BuiltinType(dataType, chain); ok {
		name := TypeIDName(builtin)
		result.BuiltinType = &name
	}

	return result, nil
}

// SuperTypes walks the HasSubtype references upwards and returns the inheritance
// chain of a type node, nearest ancestor first.
func SuperTypes(ctx context.Context, client Reader, node *ua.NodeID) ([]*ua.NodeID, error) {
	var chain []*ua.NodeID

	current := node
	seen := map[string]bool{node.String(): true}

	// A type hierarchy is shallow in practice; the bound only exists so a
	// server with a cyclic hierarchy cannot spin this for ever.
	for range 32 {
		response, err := client.Browse(ctx, &ua.BrowseRequest{
			NodesToBrowse: []*ua.BrowseDescription{{
				NodeID:          current,
				BrowseDirection: ua.BrowseDirectionInverse,
				ReferenceTypeID: ua.NewNumericNodeID(0, id.HasSubtype),
				IncludeSubtypes: false,
				NodeClassMask:   uint32(ua.NodeClassAll),
				ResultMask:      uint32(ua.BrowseResultMaskAll),
			}},
		})
		if err != nil {
			return chain, err
		}
		if len(response.Results) == 0 || response.Results[0].StatusCode != ua.StatusOK {
			return chain, nil
		}

		var parent *ua.NodeID
		for _, reference := range response.Results[0].References {
			if reference.NodeID != nil && reference.NodeID.NodeID != nil {
				parent = reference.NodeID.NodeID
				break
			}
		}
		if parent == nil || seen[parent.String()] {
			return chain, nil
		}

		chain = append(chain, parent)
		seen[parent.String()] = true
		current = parent
	}

	return chain, nil
}

// SubTypes lists the direct subtypes of a type node.
func SubTypes(ctx context.Context, client Reader, node *ua.NodeID) ([]cligen.Node, error) {
	return Browse(ctx, client, BrowseOptions{
		Root:            node,
		Depth:           1,
		Direction:       ua.BrowseDirectionForward,
		ReferenceType:   ua.NewNumericNodeID(0, id.HasSubtype),
		IncludeSubtypes: false,
	})
}

// BuiltinType finds the built-in type a data type derives from, by looking for a
// standard numeric identifier in the type's own id and then in its inheritance
// chain. The built-in types occupy the first identifiers of namespace 0, which is
// what makes this recognisable without reading anything further.
func BuiltinType(dataType *ua.NodeID, chain []*ua.NodeID) (ua.TypeID, bool) {
	candidates := append([]*ua.NodeID{dataType}, chain...)
	for _, candidate := range candidates {
		if candidate == nil || candidate.Namespace() != 0 {
			continue
		}
		if builtin, ok := builtinFromID(candidate.IntID()); ok {
			return builtin, true
		}
	}
	return 0, false
}

// builtinFromID maps a standard data type identifier to the built-in type it is,
// including the subtypes the specification defines in terms of one: UtcTime is a
// DateTime, Duration is a Double, an Enumeration is written as an Int32.
func builtinFromID(numeric uint32) (ua.TypeID, bool) {
	switch numeric {
	case id.Boolean:
		return ua.TypeIDBoolean, true
	case id.SByte:
		return ua.TypeIDSByte, true
	case id.Byte:
		return ua.TypeIDByte, true
	case id.Int16:
		return ua.TypeIDInt16, true
	case id.UInt16:
		return ua.TypeIDUint16, true
	case id.Int32, id.Enumeration:
		return ua.TypeIDInt32, true
	case id.UInt32:
		return ua.TypeIDUint32, true
	case id.Int64:
		return ua.TypeIDInt64, true
	case id.UInt64:
		return ua.TypeIDUint64, true
	case id.Float:
		return ua.TypeIDFloat, true
	case id.Double, id.Duration:
		return ua.TypeIDDouble, true
	case id.String, id.NumericRange, id.NormalizedString, id.DecimalString, id.DurationString, id.TimeString, id.DateString:
		return ua.TypeIDString, true
	case id.DateTime, id.UtcTime:
		return ua.TypeIDDateTime, true
	case id.GUID:
		return ua.TypeIDGUID, true
	case id.ByteString, id.Image, id.AudioDataType:
		return ua.TypeIDByteString, true
	case id.XMLElement:
		return ua.TypeIDXMLElement, true
	case id.NodeID:
		return ua.TypeIDNodeID, true
	case id.ExpandedNodeID:
		return ua.TypeIDExpandedNodeID, true
	case id.StatusCode:
		return ua.TypeIDStatusCode, true
	case id.QualifiedName:
		return ua.TypeIDQualifiedName, true
	case id.LocalizedText:
		return ua.TypeIDLocalizedText, true
	case id.Structure:
		return ua.TypeIDExtensionObject, true
	case id.Number, id.Integer, id.UInteger:
		// The abstract numeric types carry no encoding of their own; a value
		// declared with one is written as a Double, which every numeric type
		// converts to without loss of meaning for a command line write.
		return ua.TypeIDDouble, true
	default:
		return 0, false
	}
}

// Describe reads a type node and reports what a client needs in order to work
// with values of that type: where it sits in the hierarchy, and the fields the
// server publishes for a structure or an enumeration.
func Describe(ctx context.Context, client Reader, node *ua.NodeID, withFields bool) (cligen.TypeInfo, error) {
	set, err := Attributes(ctx, client, node, false)
	if err != nil {
		return cligen.TypeInfo{}, err
	}

	info := cligen.TypeInfo{
		NodeID:      node.String(),
		BrowseName:  typeNodeName(node),
		NodeClass:   set.NodeClass,
		DisplayName: set.DisplayName,
		Kind:        "type",
	}
	if set.BrowseName != nil && *set.BrowseName != "" {
		info.BrowseName = *set.BrowseName
	}
	for _, attribute := range set.Attributes {
		switch attribute.Name {
		case "Description":
			info.Description = attribute.Text
		case "IsAbstract":
			if abstract, ok := attribute.Value.(bool); ok {
				info.IsAbstract = &abstract
			}
		}
	}

	chain, err := SuperTypes(ctx, client, node)
	if err == nil {
		for _, ancestor := range chain {
			info.SuperTypes = append(info.SuperTypes, typeNodeName(ancestor))
		}
	}

	if subTypes, err := SubTypes(ctx, client, node); err == nil {
		for _, subType := range subTypes {
			info.SubTypes = append(info.SubTypes, subType.BrowseName)
		}
	}

	if encodings, err := Browse(ctx, client, BrowseOptions{
		Root:          node,
		Depth:         1,
		Direction:     ua.BrowseDirectionForward,
		ReferenceType: ua.NewNumericNodeID(0, id.HasEncoding),
	}); err == nil {
		for _, encoding := range encodings {
			info.Encodings = append(info.Encodings, encoding.BrowseName)
		}
	}

	info.Kind = typeKind(node, chain, info)

	if withFields {
		fields, kind, err := typeFields(ctx, client, node)
		if err == nil && len(fields) > 0 {
			info.Fields = fields
			if kind != "" {
				info.Kind = kind
			}
		}
	}

	return info, nil
}

// typeKind classifies a type for the summary line: whether values of it are a
// plain built-in, a structure, an enumeration, or an abstract placeholder.
func typeKind(node *ua.NodeID, chain []*ua.NodeID, info cligen.TypeInfo) string {
	if node.Namespace() == 0 {
		if _, ok := builtinFromID(node.IntID()); ok {
			return "builtin"
		}
	}

	for _, ancestor := range append([]*ua.NodeID{node}, chain...) {
		if ancestor == nil || ancestor.Namespace() != 0 {
			continue
		}
		switch ancestor.IntID() {
		case id.Enumeration:
			return "enumeration"
		case id.Structure:
			return "structure"
		case id.OptionSet:
			return "option-set"
		}
	}

	if info.IsAbstract != nil && *info.IsAbstract {
		return "abstract"
	}
	return "type"
}

// typeFields reads the DataTypeDefinition attribute, which is how a server
// publishes the layout of a structure or the members of an enumeration. Without
// it a client cannot decode a custom structure at all, so its absence is worth
// reporting rather than hiding.
func typeFields(ctx context.Context, client Reader, node *ua.NodeID) ([]cligen.TypeField, string, error) {
	value, err := ReadOne(ctx, client, node, ua.AttributeIDDataTypeDefinition)
	if err != nil {
		return nil, "", err
	}

	object, ok := value.Value().(*ua.ExtensionObject)
	if !ok || object == nil {
		return nil, "", fmt.Errorf("DataTypeDefinition is %s, expected a structure", TypeName(value))
	}

	switch definition := object.Value.(type) {
	case *ua.StructureDefinition:
		fields := make([]cligen.TypeField, 0, len(definition.Fields))
		for _, field := range definition.Fields {
			if field == nil {
				continue
			}
			entry := cligen.TypeField{Name: field.Name}
			if field.DataType != nil {
				entry.DataType = new(typeNodeName(field.DataType))
			}
			rank := int(field.ValueRank)
			entry.ValueRank = &rank
			entry.IsOptional = new(field.IsOptional)
			if field.MaxStringLength > 0 {
				entry.MaxStringLength = new(int(field.MaxStringLength))
			}
			if field.Description != nil && field.Description.Text != "" {
				entry.Description = new(field.Description.Text)
			}
			fields = append(fields, entry)
		}
		return fields, "structure", nil

	case *ua.EnumDefinition:
		fields := make([]cligen.TypeField, 0, len(definition.Fields))
		for _, field := range definition.Fields {
			if field == nil {
				continue
			}
			entry := cligen.TypeField{Name: field.Name, Value: new(int(field.Value))}
			if field.DisplayName != nil && field.DisplayName.Text != "" && field.DisplayName.Text != field.Name {
				entry.Description = new(field.DisplayName.Text)
			}
			if field.Description != nil && field.Description.Text != "" {
				entry.Description = new(field.Description.Text)
			}
			fields = append(fields, entry)
		}
		return fields, "enumeration", nil

	default:
		return nil, "", fmt.Errorf("unsupported DataTypeDefinition %T", object.Value)
	}
}

// TypeFolder is the root of one of the type hierarchies a server publishes.
func TypeFolder(kind string) (*ua.NodeID, error) {
	switch kind {
	case "data", "":
		return ua.NewNumericNodeID(0, id.DataTypesFolder), nil
	case "object":
		return ua.NewNumericNodeID(0, id.ObjectTypesFolder), nil
	case "variable":
		return ua.NewNumericNodeID(0, id.VariableTypesFolder), nil
	case "reference":
		return ua.NewNumericNodeID(0, id.ReferenceTypesFolder), nil
	case "event":
		return ua.NewNumericNodeID(0, id.BaseEventType), nil
	default:
		return nil, fmt.Errorf("unknown type hierarchy %q (known: data, object, variable, reference, event)", kind)
	}
}

// typeNodeName is the readable name of a type node: the specification's name for
// a standard type, and the node id for a server-defined one, which at least
// identifies it.
func typeNodeName(node *ua.NodeID) string {
	if node == nil {
		return ""
	}
	if name := StandardName(node); name != "" {
		return name
	}
	return node.String()
}
