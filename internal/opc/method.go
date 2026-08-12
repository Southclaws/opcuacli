package opc

import (
	"context"
	"fmt"
	"strings"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// Caller is the subset of the client needed to call a method.
type Caller interface {
	Reader
	Call(context.Context, *ua.CallMethodRequest) (*ua.CallMethodResult, error)
}

// Method is a method node together with the signature the server publishes for
// it, which is what makes typed argument parsing possible.
type Method struct {
	Object *ua.NodeID
	Node   *ua.NodeID
	Name   string
	Inputs []Argument
	Output []Argument
}

// Argument is one declared input or output argument of a method.
type Argument struct {
	Name        string
	DataType    *ua.NodeID
	ValueRank   int32
	Description string
	// Builtin is the built-in type an argument value is encoded as, resolved
	// from its data type.
	Builtin ua.TypeID
	// HasBuiltin is false for an argument whose data type resolves to no
	// built-in type, such as a custom structure, which this tool cannot build
	// from command line text.
	HasBuiltin bool
}

// FindMethod locates a method on an object, by node id or by browse name, and
// reads its InputArguments and OutputArguments properties.
//
// Reading the signature first is what lets a call parse "21.5" into the Double
// the method declares, and catch a wrong argument count before the server does.
func FindMethod(ctx context.Context, client Reader, object *ua.NodeID, method string) (*Method, error) {
	node, err := methodNode(ctx, client, object, method)
	if err != nil {
		return nil, err
	}

	found := &Method{Object: object, Node: node.NodeID, Name: node.Name}

	properties, err := Browse(ctx, client, BrowseOptions{
		Root:            node.NodeID,
		Depth:           1,
		Direction:       ua.BrowseDirectionForward,
		ReferenceType:   ua.NewNumericNodeID(0, id.HasProperty),
		IncludeSubtypes: true,
	})
	if err != nil {
		return nil, fmt.Errorf("read method properties: %w", err)
	}

	for _, property := range properties {
		propertyNode, err := ParseNodeID(property.NodeID)
		if err != nil {
			continue
		}
		switch property.BrowseName {
		case "InputArguments":
			found.Inputs, err = readArguments(ctx, client, propertyNode)
		case "OutputArguments":
			found.Output, err = readArguments(ctx, client, propertyNode)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
	}

	return found, nil
}

// methodNode resolves the method argument, which may be a node id or a browse
// name of a method on the object.
type namedNode struct {
	NodeID *ua.NodeID
	Name   string
}

func methodNode(ctx context.Context, client Reader, object *ua.NodeID, method string) (namedNode, error) {
	methods, err := Browse(ctx, client, BrowseOptions{
		Root:            object,
		Depth:           1,
		Direction:       ua.BrowseDirectionForward,
		ReferenceType:   ua.NewNumericNodeID(0, id.HasComponent),
		IncludeSubtypes: true,
		ClassMask:       ua.NodeClassMethod,
	})
	if err != nil {
		return namedNode{}, fmt.Errorf("browse methods of %s: %w", object, err)
	}

	// A node id is unambiguous, so match on it first; then fall back to the
	// browse name, which is how a person refers to a method.
	if parsed, err := ParseNodeID(method); err == nil {
		for _, candidate := range methods {
			if candidate.NodeID == parsed.String() {
				return namedNode{NodeID: parsed, Name: candidate.BrowseName}, nil
			}
		}
		// A method may be reachable by a reference this listing did not follow,
		// so an explicit node id is trusted even when it is not in the list.
		if looksLikeNodeID(method) {
			return namedNode{NodeID: parsed, Name: method}, nil
		}
	}

	for _, candidate := range methods {
		if candidate.BrowseName == method || trimNamespace(candidate.BrowseName) == method {
			parsed, err := ParseNodeID(candidate.NodeID)
			if err != nil {
				return namedNode{}, err
			}
			return namedNode{NodeID: parsed, Name: candidate.BrowseName}, nil
		}
	}

	available := make([]string, 0, len(methods))
	for _, candidate := range methods {
		available = append(available, candidate.BrowseName)
	}
	if len(available) == 0 {
		return namedNode{}, fmt.Errorf("object %s has no methods", object)
	}
	return namedNode{}, fmt.Errorf("object %s has no method %q (available: %v)", object, method, available)
}

// readArguments reads an InputArguments or OutputArguments property, whose value
// is an array of Argument structures.
func readArguments(ctx context.Context, client Reader, property *ua.NodeID) ([]Argument, error) {
	value, err := ReadOne(ctx, client, property, ua.AttributeIDValue)
	if err != nil {
		return nil, err
	}

	objects, ok := value.Value().([]*ua.ExtensionObject)
	if !ok {
		return nil, nil
	}

	arguments := make([]Argument, 0, len(objects))
	for _, object := range objects {
		if object == nil {
			continue
		}
		declared, ok := object.Value.(*ua.Argument)
		if !ok {
			continue
		}

		argument := Argument{
			Name:      declared.Name,
			DataType:  declared.DataType,
			ValueRank: declared.ValueRank,
		}
		if declared.Description != nil {
			argument.Description = declared.Description.Text
		}
		// The chain is not walked here: an argument's data type is almost
		// always a built-in one, and a call should not cost a browse per
		// argument. A custom type is reported as unresolved so the caller can
		// ask for an explicit --arg-type.
		if builtin, ok := BuiltinType(declared.DataType, nil); ok {
			argument.Builtin = builtin
			argument.HasBuiltin = true
		}
		arguments = append(arguments, argument)
	}

	return arguments, nil
}

// Signature renders a method's declared signature the way a person reads one.
func (m *Method) Signature() string {
	inputs := make([]string, 0, len(m.Inputs))
	for _, argument := range m.Inputs {
		inputs = append(inputs, fmt.Sprintf("%s %s", argument.Name, argument.TypeName()))
	}
	outputs := make([]string, 0, len(m.Output))
	for _, argument := range m.Output {
		outputs = append(outputs, fmt.Sprintf("%s %s", argument.Name, argument.TypeName()))
	}

	signature := fmt.Sprintf("%s(%s)", m.Name, strings.Join(inputs, ", "))
	if len(outputs) > 0 {
		signature += " -> " + strings.Join(outputs, ", ")
	}
	return signature
}

// TypeName is the argument's declared type, with a suffix for an array.
func (a Argument) TypeName() string {
	name := typeNodeName(a.DataType)
	if a.ValueRank > 0 || a.ValueRank == -3 {
		name += "[]"
	}
	return name
}

// Call invokes a method with the given input variants and reports the outputs
// against their declared names.
func Call(ctx context.Context, client Caller, method *Method, inputs []*ua.Variant) (cligen.CallResult, error) {
	result := cligen.CallResult{
		Object:     method.Object.String(),
		Method:     method.Node.String(),
		MethodName: new(method.Name),
	}

	response, err := client.Call(ctx, &ua.CallMethodRequest{
		ObjectID:       method.Object,
		MethodID:       method.Node,
		InputArguments: inputs,
	})
	if err != nil {
		return result, fmt.Errorf("call %s: %w", method.Name, err)
	}

	result.Status = StatusName(response.StatusCode)
	result.Ok = response.StatusCode == ua.StatusOK

	for index, argument := range method.Inputs {
		entry := cligen.MethodArgument{Name: argument.Name, DataType: argument.TypeName()}
		if argument.Description != "" {
			entry.Description = new(argument.Description)
		}
		if index < len(inputs) {
			entry.Value = JSON(inputs[index])
		}
		result.Inputs = append(result.Inputs, entry)
	}

	// An argument the server refused is reported per argument, which is the
	// only way to tell which one it objected to.
	for _, status := range response.InputArgumentResults {
		if status != ua.StatusOK {
			result.InputStatuses = append(result.InputStatuses, StatusName(status))
		}
	}

	for index, output := range response.OutputArguments {
		entry := cligen.MethodArgument{Name: fmt.Sprintf("out%d", index), DataType: TypeName(output)}
		if index < len(method.Output) {
			entry.Name = method.Output[index].Name
			entry.DataType = method.Output[index].TypeName()
			if method.Output[index].Description != "" {
				entry.Description = new(method.Output[index].Description)
			}
		}
		entry.Value = JSON(output)
		result.Outputs = append(result.Outputs, entry)
	}

	if !result.Ok {
		return result, fmt.Errorf("method %s returned %s: %s", method.Name, result.Status, StatusDescription(response.StatusCode))
	}

	return result, nil
}

// ParseArguments turns command line argument text into variants, using the
// method's declared types unless an override names one. It fails when the count
// does not match the signature, which the server would otherwise reject after
// the fact.
func ParseArguments(method *Method, texts, overrides []string, asJSON bool) ([]*ua.Variant, error) {
	if len(texts) != len(method.Inputs) {
		return nil, fmt.Errorf("method %s takes %d argument(s), got %d: %s",
			method.Name, len(method.Inputs), len(texts), method.Signature())
	}

	variants := make([]*ua.Variant, 0, len(texts))
	for index, text := range texts {
		declared := method.Inputs[index]

		typeID := declared.Builtin
		resolved := declared.HasBuiltin
		if index < len(overrides) && overrides[index] != "" {
			parsed, err := ParseTypeName(overrides[index])
			if err != nil {
				return nil, fmt.Errorf("argument %s: %w", declared.Name, err)
			}
			typeID, resolved = parsed, true
		}
		if !resolved {
			return nil, fmt.Errorf("argument %s has data type %s, which cannot be built from text; pass --arg-type",
				declared.Name, declared.TypeName())
		}

		isArray := declared.ValueRank > 0 || declared.ValueRank == -3
		variant, err := ParseValue(text, typeID, isArray && !asJSON, asJSON)
		if err != nil {
			return nil, fmt.Errorf("argument %s: %w", declared.Name, err)
		}
		variants = append(variants, variant)
	}

	return variants, nil
}

// looksLikeNodeID reports whether text is spelled as a node id rather than a
// browse name, so an id the browse listing missed is still accepted.
func looksLikeNodeID(text string) bool {
	for _, prefix := range []string{"i=", "s=", "g=", "b=", "ns="} {
		if len(text) >= len(prefix) && text[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// trimNamespace drops a "2:" style namespace prefix from a browse name so a user
// may name a method without it.
func trimNamespace(name string) string {
	for index := range name {
		if name[index] == ':' {
			return name[index+1:]
		}
		if name[index] < '0' || name[index] > '9' {
			return name
		}
	}
	return name
}
