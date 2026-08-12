package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gopcua/opcua/ua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// read reads attributes of nodes, in one service call for the whole request.
func read(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ReadParams) (cligen.ReadResultList, error) {
	targets, err := nodes(p.Node, p.Stdin, commandIO.In)
	if err != nil {
		return nil, err
	}

	attributes := make([]ua.AttributeID, 0, len(p.Attribute))
	for _, name := range p.Attribute {
		attribute, err := opc.ParseAttribute(name)
		if err != nil {
			return nil, err
		}
		attributes = append(attributes, attribute)
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	results, err := opc.Read(ctx, session.Client, opc.ReadOptions{
		Nodes:      targets,
		Attributes: attributes,
		IndexRange: p.IndexRange,
		MaxAge:     p.MaxAge,
		Timestamps: opc.TimestampsToReturn(string(p.Timestamps)),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return results, nil
	}

	switch format {
	case "jsonl":
		encoder := json.NewEncoder(session.Out.Raw())
		for _, result := range results {
			if err := encoder.Encode(result); err != nil {
				return nil, err
			}
		}

	case "plain":
		rows := make([][]string, 0, len(results))
		for _, result := range results {
			rows = append(rows, []string{result.NodeID, result.Attribute, derefOr(result.Text, ""), result.Status})
		}
		session.Out.Plain(rows)

	default:
		renderReadTable(session.Out, results, string(p.Timestamps))
	}

	if bad := badCount(results); bad > 0 && bad == len(results) {
		return results, fmt.Errorf("%w: every read failed", ErrService)
	}
	return results, nil
}

func renderReadTable(p *render.Printer, results cligen.ReadResultList, timestamps string) {
	headers := []string{"NODE ID", "ATTRIBUTE", "TYPE", "VALUE", "STATUS"}
	withTime := timestamps != "neither"
	if withTime {
		headers = append(headers, "TIMESTAMP")
	}

	rows := make([][]string, 0, len(results))
	for _, result := range results {
		row := []string{
			result.NodeID,
			result.Attribute,
			p.T.Styles.Type.Render(derefOr(result.Type, "")),
			render.Truncate(derefOr(result.Text, ""), 60),
			statusLabel(p, result.Status),
		}
		if withTime {
			row = append(row, p.T.Styles.Faint.Render(timestampText(result)))
		}
		rows = append(rows, row)
	}

	p.Table(headers, rows)
}

// timestampText prefers the source timestamp, which is when the value was
// produced, over the server timestamp, which is only when it was served.
func timestampText(result cligen.ReadResult) string {
	stamp := result.SourceTimestamp
	if stamp == nil {
		stamp = result.ServerTimestamp
	}
	if stamp == nil {
		return ""
	}
	return stamp.Local().Format("15:04:05.000")
}

// statusLabel colours a status code by severity, so a bad or uncertain value
// cannot be mistaken for a good one.
func statusLabel(p *render.Printer, status string) string {
	switch {
	case strings.HasPrefix(status, "Bad"):
		return p.T.Styles.Bad.Render(status)
	case strings.HasPrefix(status, "Uncertain"):
		return p.T.Styles.Uncertain.Render(status)
	default:
		return p.T.Styles.Good.Render(status)
	}
}

func badCount(results cligen.ReadResultList) int {
	bad := 0
	for _, result := range results {
		if strings.HasPrefix(result.Status, "Bad") {
			bad++
		}
	}
	return bad
}

// write writes a value to a node.
//
// The value's type is resolved from the node's own DataType attribute unless
// --type names one, because a server rejects a write whose variant type does not
// match and the resulting BadTypeMismatch says nothing about what was expected.
func write(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.WriteParams) (cligen.WriteResult, error) {
	target, err := node(p.Node)
	if err != nil {
		return cligen.WriteResult{}, err
	}
	attribute, err := opc.ParseAttribute(p.Attribute)
	if err != nil {
		return cligen.WriteResult{}, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.WriteResult{}, err
	}
	defer session.Close(ctx)

	typeID, err := writeType(ctx, session, target, p.Type)
	if err != nil {
		return cligen.WriteResult{}, err
	}

	variant, err := opc.ParseValue(p.Value, typeID, p.Array, p.Json)
	if err != nil {
		return cligen.WriteResult{}, err
	}

	result := cligen.WriteResult{
		NodeID:    target.String(),
		Attribute: opc.AttributeName(attribute),
		Type:      new(opc.TypeIDName(typeID)),
		Value:     opc.JSON(variant),
	}

	format := string(p.Format)
	human := !machineReadable(format)

	if p.DryRun {
		result.DryRun = new(true)
		result.Ok = true
		result.Status = "DryRun"
		if human {
			session.Out.Notef("would write %s = %s (%s)",
				target, opc.Text(variant), opc.TypeIDName(typeID))
		}
		return result, nil
	}

	if !p.Yes && human {
		confirmed, err := confirm(fmt.Sprintf("Write %s = %s to %s?",
			opc.TypeIDName(typeID), opc.Text(variant), target))
		if err != nil {
			return result, err
		}
		if !confirmed {
			result.Status = "Cancelled"
			return result, fmt.Errorf("cancelled")
		}
	}

	response, err := session.Client.Write(ctx, &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{{
			NodeID:      target,
			AttributeID: attribute,
			IndexRange:  p.IndexRange,
			Value:       &ua.DataValue{EncodingMask: ua.DataValueValue, Value: variant},
		}},
	})
	if err != nil {
		return result, fmt.Errorf("%w: write: %w", ErrService, err)
	}
	if len(response.Results) == 0 {
		return result, fmt.Errorf("%w: server returned no write result", ErrService)
	}

	status := response.Results[0]
	result.Status = opc.StatusName(status)
	result.Ok = status == ua.StatusOK

	if !result.Ok {
		return result, fmt.Errorf("%w: %s: %s", ErrService, result.Status, opc.StatusDescription(status))
	}
	if human {
		session.Out.Okf("%s = %s", target, opc.Text(variant))
	}

	return result, nil
}

// writeType decides which built-in type a value is written as: the one named by
// --type, or the one the node's data type resolves to.
func writeType(ctx context.Context, session *session, target *ua.NodeID, requested string) (ua.TypeID, error) {
	if requested != "" && !strings.EqualFold(requested, "auto") {
		return opc.ParseTypeName(requested)
	}

	declared, err := opc.TypeOf(ctx, session.Client, target)
	if err != nil {
		return 0, fmt.Errorf("cannot resolve the data type of %s: %w (pass --type to write anyway)", target, err)
	}
	if declared.BuiltinType == nil {
		return 0, fmt.Errorf("cannot infer a write type for %s: its data type %s has no built-in encoding; pass --type",
			target, declared.DataType)
	}

	typeID, err := opc.ParseTypeName(*declared.BuiltinType)
	if err != nil {
		return 0, fmt.Errorf("cannot infer a write type for %s: its data type %s cannot be written from text; pass --type",
			target, declared.DataType)
	}

	if session.Settings.Verbosity > 0 {
		session.Err.Notef("%s has data type %s, writing as %s", target, declared.DataType, *declared.BuiltinType)
	}
	return typeID, nil
}

// call invokes a method, parsing its arguments into the types it declares.
func call(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.CallParams) (cligen.CallResult, error) {
	object, err := node(p.Object)
	if err != nil {
		return cligen.CallResult{}, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.CallResult{}, err
	}
	defer session.Close(ctx)

	method, err := opc.FindMethod(ctx, session.Client, object, p.Method)
	if err != nil {
		return cligen.CallResult{}, fmt.Errorf("%w: %w", ErrNotFound, err)
	}

	format := string(p.Format)
	human := !machineReadable(format)

	if p.Describe {
		result := cligen.CallResult{
			Object:     object.String(),
			Method:     method.Node.String(),
			MethodName: new(method.Name),
			Status:     "NotCalled",
		}
		for _, argument := range method.Inputs {
			result.Inputs = append(result.Inputs, describeArgument(argument))
		}
		for _, argument := range method.Output {
			result.Outputs = append(result.Outputs, describeArgument(argument))
		}
		if human {
			renderSignature(session.Out, method)
		}
		return result, nil
	}

	inputs, err := opc.ParseArguments(method, p.Argument, p.ArgType, p.Json)
	if err != nil {
		return cligen.CallResult{}, err
	}

	result, err := opc.Call(ctx, session.Client, method, inputs)
	if err != nil {
		return result, fmt.Errorf("%w: %w", ErrService, err)
	}

	if human {
		session.Out.Okf("%s returned %s", method.Name, result.Status)
		if len(result.Outputs) > 0 {
			fields := make([]render.Field, 0, len(result.Outputs))
			for _, output := range result.Outputs {
				fields = append(fields, render.F(output.Name, fmt.Sprint(output.Value)))
			}
			session.Out.KV(fields)
		}
	}

	return result, nil
}

func describeArgument(argument opc.Argument) cligen.MethodArgument {
	described := cligen.MethodArgument{Name: argument.Name, DataType: argument.TypeName()}
	if argument.Description != "" {
		described.Description = new(argument.Description)
	}
	rank := int(argument.ValueRank)
	described.ValueRank = &rank
	return described
}

func renderSignature(p *render.Printer, method *opc.Method) {
	p.Title(method.Signature(), method.Node.String())

	if len(method.Inputs) == 0 && len(method.Output) == 0 {
		p.Notef("this method takes no arguments and returns nothing")
		return
	}

	rows := make([][]string, 0, len(method.Inputs)+len(method.Output))
	for _, argument := range method.Inputs {
		rows = append(rows, []string{"in", argument.Name, argument.TypeName(), argument.Description})
	}
	for _, argument := range method.Output {
		rows = append(rows, []string{"out", argument.Name, argument.TypeName(), argument.Description})
	}
	p.Table([]string{"DIR", "NAME", "TYPE", "DESCRIPTION"}, rows)
}

// confirm asks a yes/no question on the terminal. A non-interactive stream is
// treated as a refusal rather than a silent yes, because a scripted write should
// pass --yes deliberately.
func confirm(question string) (bool, error) {
	answer, err := promptConfirm(question)
	if err != nil {
		return false, fmt.Errorf("%s (pass --yes to skip the prompt)", err)
	}
	return answer, nil
}
