package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gopcua/opcua/ua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// attributes shows every attribute of a node, which is the fastest way to
// understand an unfamiliar one.
func attributes(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.AttributesParams) (cligen.AttributeSet, error) {
	target, err := node(p.Node)
	if err != nil {
		return cligen.AttributeSet{}, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.AttributeSet{}, err
	}
	defer session.Close(ctx)

	set, err := opc.Attributes(ctx, session.Client, target, p.All)
	if err != nil {
		return cligen.AttributeSet{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return set, nil
	}

	rows := make([][]string, 0, len(set.Attributes))
	for _, attribute := range set.Attributes {
		rows = append(rows, []string{
			attribute.Name,
			render.Truncate(derefOr(attribute.Text, ""), 70),
			statusLabel(session.Out, attribute.Status),
		})
	}

	if format == "plain" {
		plain := make([][]string, 0, len(set.Attributes))
		for _, attribute := range set.Attributes {
			plain = append(plain, []string{attribute.Name, derefOr(attribute.Text, ""), attribute.Status})
		}
		session.Out.Plain(plain)
		return set, nil
	}

	subtitle := set.NodeClass
	if set.DisplayName != nil {
		subtitle = fmt.Sprintf("%s · %s", *set.DisplayName, set.NodeClass)
	}
	session.Out.Title(set.NodeID, subtitle)
	session.Out.Table([]string{"ATTRIBUTE", "VALUE", "STATUS"}, rows)
	return set, nil
}

// typeGet describes a data type, including the fields a structure or enumeration
// publishes.
func typeGet(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.TypeGetParams) (cligen.TypeInfo, error) {
	target, err := node(p.Node)
	if err != nil {
		return cligen.TypeInfo{}, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.TypeInfo{}, err
	}
	defer session.Close(ctx)

	info, err := opc.Describe(ctx, session.Client, target, !p.NoFields)
	if err != nil {
		return cligen.TypeInfo{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return info, nil
	}

	if format == "markdown" {
		return info, renderMarkdown(session.Out, typeMarkdown(info))
	}

	session.Out.Title(info.BrowseName, fmt.Sprintf("%s · %s", info.NodeID, info.Kind))

	fields := []render.Field{
		render.F("Node class", info.NodeClass),
		render.F("Kind", info.Kind),
	}
	if info.IsAbstract != nil {
		fields = append(fields, render.F("Abstract", strconv.FormatBool(*info.IsAbstract)))
	}
	if info.Description != nil {
		fields = append(fields, render.F("Description", *info.Description))
	}
	if len(info.SuperTypes) > 0 {
		fields = append(fields, render.F("Inherits", strings.Join(info.SuperTypes, " "+session.Out.T.Symbols.Arrow+" ")))
	}
	if len(info.Encodings) > 0 {
		fields = append(fields, render.F("Encodings", strings.Join(info.Encodings, ", ")))
	}
	if len(info.SubTypes) > 0 {
		fields = append(fields, render.F("Subtypes", render.Truncate(strings.Join(info.SubTypes, ", "), session.Out.Width-20)))
	}
	session.Out.KV(fields)

	if len(info.Fields) == 0 {
		if info.Kind == "structure" || info.Kind == "enumeration" {
			session.Out.Println()
			session.Err.Warnf("the server publishes no DataTypeDefinition, so values of this type cannot be decoded")
		}
		return info, nil
	}

	session.Out.Println()
	if info.Kind == "enumeration" {
		rows := make([][]string, 0, len(info.Fields))
		for _, field := range info.Fields {
			value := ""
			if field.Value != nil {
				value = strconv.Itoa(*field.Value)
			}
			rows = append(rows, []string{value, field.Name, derefOr(field.Description, "")})
		}
		session.Out.Table([]string{"VALUE", "NAME", "DESCRIPTION"}, rows)
		return info, nil
	}

	rows := make([][]string, 0, len(info.Fields))
	for _, field := range info.Fields {
		optional := ""
		if field.IsOptional != nil && *field.IsOptional {
			optional = "optional"
		}
		rows = append(rows, []string{
			field.Name,
			derefOr(field.DataType, ""),
			rankSuffix(field.ValueRank),
			optional,
			derefOr(field.Description, ""),
		})
	}
	session.Out.Table([]string{"FIELD", "TYPE", "RANK", "", "DESCRIPTION"}, rows)
	return info, nil
}

func rankSuffix(rank *int) string {
	if rank == nil {
		return ""
	}
	return opc.ValueRankText(int32(*rank))
}

// typeMarkdown renders a type as Markdown, for pasting into documentation.
func typeMarkdown(info cligen.TypeInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", info.BrowseName)
	fmt.Fprintf(&b, "`%s` - %s\n\n", info.NodeID, info.Kind)
	if info.Description != nil {
		fmt.Fprintf(&b, "%s\n\n", *info.Description)
	}
	if len(info.SuperTypes) > 0 {
		fmt.Fprintf(&b, "**Inherits:** %s\n\n", strings.Join(info.SuperTypes, " → "))
	}

	if len(info.Fields) > 0 {
		if info.Kind == "enumeration" {
			b.WriteString("| Value | Name | Description |\n|---|---|---|\n")
			for _, field := range info.Fields {
				value := ""
				if field.Value != nil {
					value = strconv.Itoa(*field.Value)
				}
				fmt.Fprintf(&b, "| %s | %s | %s |\n", value, field.Name, derefOr(field.Description, ""))
			}
		} else {
			b.WriteString("| Field | Type | Rank | Description |\n|---|---|---|---|\n")
			for _, field := range info.Fields {
				fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
					field.Name, derefOr(field.DataType, ""), rankSuffix(field.ValueRank), derefOr(field.Description, ""))
			}
		}
		b.WriteString("\n")
	}

	if len(info.SubTypes) > 0 {
		fmt.Fprintf(&b, "**Subtypes:** %s\n", strings.Join(info.SubTypes, ", "))
	}
	return b.String()
}

// typeOf reports the declared type of a variable.
func typeOf(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.TypeOfParams) (cligen.TypeOf, error) {
	target, err := node(p.Node)
	if err != nil {
		return cligen.TypeOf{}, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.TypeOf{}, err
	}
	defer session.Close(ctx)

	declared, err := opc.TypeOf(ctx, session.Client, target)
	if err != nil {
		return cligen.TypeOf{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return declared, nil
	}

	if format == "table" {
		dimensions := make([]string, 0, len(declared.ArrayDimensions))
		for _, size := range declared.ArrayDimensions {
			dimensions = append(dimensions, strconv.Itoa(size))
		}
		session.Out.Table(
			[]string{"NODE ID", "DATA TYPE", "DATA TYPE ID", "BUILT-IN", "RANK", "DIMENSIONS"},
			[][]string{{
				declared.NodeID,
				declared.DataType,
				declared.DataTypeID,
				derefOr(declared.BuiltinType, ""),
				derefOr(declared.Rank, ""),
				strings.Join(dimensions, "×"),
			}})
		return declared, nil
	}

	fmt.Fprintln(session.Out.Raw(), declared.DataType)
	return declared, nil
}

// typeList walks one of the type hierarchies a server publishes.
func typeList(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.TypeListParams) (cligen.NodeList, error) {
	root, err := opc.TypeFolder(string(p.Kind))
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

	// An event type hierarchy is built from HasSubtype references rather than
	// the hierarchical ones that organise a folder, so the reference followed
	// depends on which hierarchy was asked for.
	referenceType := ua.NewNumericNodeID(0, 33) // HierarchicalReferences
	if p.Kind == cligen.TypeListKindEvent {
		referenceType, _ = opc.ParseReferenceType("HasSubtype")
	}

	types, err := opc.Browse(ctx, session.Client, opc.BrowseOptions{
		Root:            root,
		Depth:           p.Depth,
		Direction:       ua.BrowseDirectionForward,
		ReferenceType:   referenceType,
		IncludeSubtypes: true,
		Match:           byNamespace,
		Limit:           p.Limit,
		Nested:          format == "tree",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	if machineReadable(format) {
		return types, nil
	}

	switch format {
	case "tree":
		label := fmt.Sprintf("%s types", string(p.Kind))
		session.Out.Tree(session.Out.T.Styles.Title.Render(label), nodeTree(session.Out, types, false))
	case "plain":
		session.Out.Plain(nodeRows(types, false, true))
	default:
		renderNodeTable(session.Out, types, false)
	}

	return types, nil
}

// namespaces lists the server's namespace array.
func namespaces(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.NamespacesParams) (cligen.NamespaceList, error) {
	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	list, err := opc.Namespaces(ctx, session.Client)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return list, nil
	}

	rows := make([][]string, 0, len(list))
	for _, namespace := range list {
		rows = append(rows, []string{strconv.Itoa(namespace.Index), namespace.URI})
	}

	if format == "plain" {
		session.Out.Plain(rows)
		return list, nil
	}

	session.Out.Title(session.Client.Endpoint, fmt.Sprintf("%d namespace(s)", len(list)))
	session.Out.Table([]string{"NS", "URI"}, rows)
	return list, nil
}
