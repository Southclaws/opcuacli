package opc

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gopcua/opcua/ua"
)

// Severity classifies a status code by the two high bits the specification
// reserves for it, which is what decides how a value is coloured.
type Severity int

const (
	// SeverityGood means the value can be used.
	SeverityGood Severity = iota
	// SeverityUncertain means the value is usable but the server has doubts
	// about it, typically a stale or substituted reading.
	SeverityUncertain
	// SeverityBad means there is no usable value.
	SeverityBad
)

// StatusSeverity classifies a status code.
func StatusSeverity(status ua.StatusCode) Severity {
	switch uint32(status) & 0xC0000000 {
	case 0x80000000:
		return SeverityBad
	case 0x40000000:
		return SeverityUncertain
	default:
		return SeverityGood
	}
}

// StatusName is the short name of a status code, without the "Status" prefix
// the generated constants carry: "Good", "BadNodeIDUnknown". An unknown code is
// rendered as hex, which is still the most useful thing to show.
func StatusName(status ua.StatusCode) string {
	if description, ok := ua.StatusCodes[status]; ok {
		return strings.TrimPrefix(description.Name, "Status")
	}
	return fmt.Sprintf("0x%08X", uint32(status))
}

// StatusDescription is the specification's sentence explaining a status code,
// for the places with room to show it.
func StatusDescription(status ua.StatusCode) string {
	if description, ok := ua.StatusCodes[status]; ok {
		return description.Text
	}
	return ""
}

// TypeName is the built-in type of a variant's value, e.g. "Double" or
// "String[]" for an array of strings. An empty variant has no type.
func TypeName(variant *ua.Variant) string {
	if variant == nil {
		return ""
	}
	name := strings.TrimPrefix(variant.Type().String(), "TypeID")
	if variant.ArrayLength() > 0 || len(variant.ArrayDimensions()) > 0 {
		name += "[]"
	}
	return name
}

// Text renders a variant the way a person reads it: a bare scalar, a
// comma-separated list for an array, and the meaningful field of the composite
// types rather than their Go struct.
func Text(variant *ua.Variant) string {
	if variant == nil {
		return ""
	}
	return valueText(variant.Value())
}

func valueText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float32:
		return formatFloat(float64(typed), 32)
	case float64:
		return formatFloat(typed, 64)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	case []byte:
		return hexOrText(typed)
	case *ua.LocalizedText:
		if typed == nil {
			return ""
		}
		return typed.Text
	case *ua.QualifiedName:
		if typed == nil {
			return ""
		}
		if typed.NamespaceIndex == 0 {
			return typed.Name
		}
		return fmt.Sprintf("%d:%s", typed.NamespaceIndex, typed.Name)
	case *ua.NodeID:
		return nodeIDText(typed)
	case *ua.ExpandedNodeID:
		if typed == nil {
			return ""
		}
		return nodeIDText(typed.NodeID)
	case ua.StatusCode:
		return StatusName(typed)
	case *ua.GUID:
		if typed == nil {
			return ""
		}
		return typed.String()
	case *ua.ExtensionObject:
		return extensionText(typed)
	case *ua.DataValue:
		if typed == nil {
			return ""
		}
		return valueText(typed.Value.Value())
	case *ua.Variant:
		return Text(typed)
	case ua.XMLElement:
		return string(typed)
	}

	// Arrays and matrices arrive as a slice of the element type, whatever that
	// is, so reflection is the only way to walk them without enumerating all
	// twenty-odd built-in types twice.
	reflected := reflect.ValueOf(value)
	if reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array {
		parts := make([]string, reflected.Len())
		for i := range parts {
			parts[i] = valueText(reflected.Index(i).Interface())
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}

	return fmt.Sprint(value)
}

func nodeIDText(node *ua.NodeID) string {
	if node == nil {
		return ""
	}
	text := node.String()
	if name := StandardName(node); name != "" {
		return fmt.Sprintf("%s (%s)", text, name)
	}
	return text
}

// extensionText names an extension object by its type rather than dumping the
// encoded body: a structure whose definition the client has not read cannot be
// rendered field by field, and its type is the useful part.
func extensionText(object *ua.ExtensionObject) string {
	if object == nil || object.Value == nil {
		return ""
	}
	if raw, ok := object.Value.([]byte); ok {
		name := "Structure"
		if object.TypeID != nil {
			if standard := StandardName(object.TypeID.NodeID); standard != "" {
				name = standard
			} else if object.TypeID.NodeID != nil {
				name = object.TypeID.NodeID.String()
			}
		}
		return fmt.Sprintf("%s(%d bytes)", name, len(raw))
	}
	return fmt.Sprintf("%T", object.Value)
}

func formatFloat(value float64, bits int) string {
	if math.IsNaN(value) {
		return "NaN"
	}
	if math.IsInf(value, 1) {
		return "+Inf"
	}
	if math.IsInf(value, -1) {
		return "-Inf"
	}
	return strconv.FormatFloat(value, 'g', -1, bits)
}

// hexOrText shows a byte string as text when it is printable and as hex when it
// is not, because both spellings occur in practice and the wrong one is useless.
func hexOrText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	printable := true
	for _, b := range raw {
		if b < 0x20 || b > 0x7E {
			printable = false
			break
		}
	}
	if printable {
		return string(raw)
	}
	if len(raw) > 32 {
		return hex.EncodeToString(raw[:32]) + "… (" + strconv.Itoa(len(raw)) + " bytes)"
	}
	return hex.EncodeToString(raw)
}

// JSON converts a variant into a value that encodes as the JSON type a reader
// expects: numbers as numbers, timestamps as RFC 3339 strings, byte strings as
// base64, and the composite types as their meaningful field.
func JSON(variant *ua.Variant) any {
	if variant == nil {
		return nil
	}
	return jsonValue(variant.Value())
}

func jsonValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case bool, string, int8, int16, int32, int64, uint8, uint16, uint32, uint64:
		return typed
	case float32:
		return jsonFloat(float64(typed))
	case float64:
		return jsonFloat(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	case []byte:
		return base64.StdEncoding.EncodeToString(typed)
	case *ua.LocalizedText:
		if typed == nil {
			return nil
		}
		return typed.Text
	case *ua.QualifiedName:
		if typed == nil {
			return nil
		}
		return valueText(typed)
	case *ua.NodeID:
		if typed == nil {
			return nil
		}
		return typed.String()
	case *ua.ExpandedNodeID:
		if typed == nil || typed.NodeID == nil {
			return nil
		}
		return typed.NodeID.String()
	case ua.StatusCode:
		return StatusName(typed)
	case *ua.GUID:
		if typed == nil {
			return nil
		}
		return typed.String()
	case *ua.ExtensionObject:
		return extensionText(typed)
	case *ua.DataValue:
		if typed == nil {
			return nil
		}
		return jsonValue(typed.Value.Value())
	case *ua.Variant:
		return JSON(typed)
	case ua.XMLElement:
		return string(typed)
	}

	reflected := reflect.ValueOf(value)
	if reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array {
		items := make([]any, reflected.Len())
		for i := range items {
			items[i] = jsonValue(reflected.Index(i).Interface())
		}
		return items
	}

	return valueText(value)
}

// jsonFloat keeps a non-finite float representable: JSON has no NaN or
// infinity, so those become their conventional strings rather than an encoding
// error that loses the whole result.
func jsonFloat(value float64) any {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return formatFloat(value, 64)
	}
	return value
}

// Numeric extracts a float from a variant for plotting, reporting whether the
// value is numeric at all.
func Numeric(variant *ua.Variant) (float64, bool) {
	if variant == nil {
		return 0, false
	}
	switch value := variant.Value().(type) {
	case bool:
		if value {
			return 1, true
		}
		return 0, true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case float32:
		return float64(value), true
	case float64:
		return value, true
	default:
		return 0, false
	}
}

// writableTypes are the built-in types a value can be written as, in the order
// help text lists them.
var writableTypes = []struct {
	Name string
	Type ua.TypeID
}{
	{"boolean", ua.TypeIDBoolean},
	{"sbyte", ua.TypeIDSByte},
	{"byte", ua.TypeIDByte},
	{"int16", ua.TypeIDInt16},
	{"uint16", ua.TypeIDUint16},
	{"int32", ua.TypeIDInt32},
	{"uint32", ua.TypeIDUint32},
	{"int64", ua.TypeIDInt64},
	{"uint64", ua.TypeIDUint64},
	{"float", ua.TypeIDFloat},
	{"double", ua.TypeIDDouble},
	{"string", ua.TypeIDString},
	{"datetime", ua.TypeIDDateTime},
	{"guid", ua.TypeIDGUID},
	{"bytestring", ua.TypeIDByteString},
	{"xmlelement", ua.TypeIDXMLElement},
	{"nodeid", ua.TypeIDNodeID},
	{"statuscode", ua.TypeIDStatusCode},
	{"qualifiedname", ua.TypeIDQualifiedName},
	{"localizedtext", ua.TypeIDLocalizedText},
}

// WritableTypeNames lists the type names --type accepts, for help text and
// shell completion.
func WritableTypeNames() []string {
	names := make([]string, 0, len(writableTypes)+1)
	names = append(names, "auto")
	for _, candidate := range writableTypes {
		names = append(names, candidate.Name)
	}
	return names
}

// ParseTypeName maps a --type value to a built-in type.
func ParseTypeName(name string) (ua.TypeID, error) {
	key := foldName(name)
	for _, candidate := range writableTypes {
		if foldName(candidate.Name) == key {
			return candidate.Type, nil
		}
	}
	return 0, fmt.Errorf("unknown type %q (known: %s)", name, strings.Join(WritableTypeNames(), ", "))
}

// TypeIDName is the readable name of a built-in type.
func TypeIDName(typeID ua.TypeID) string {
	return strings.TrimPrefix(typeID.String(), "TypeID")
}

// ParseValue turns command line text into a variant of the given built-in type.
//
// The mode matters because a server rejects a write whose variant type does not
// match the node's DataType, so the caller either names the type or resolves the
// node's own type first - this function never guesses.
func ParseValue(text string, typeID ua.TypeID, asArray, asJSON bool) (*ua.Variant, error) {
	if asJSON {
		return parseJSONValue(text, typeID)
	}
	if asArray {
		return parseArrayValue(splitArray(text), typeID)
	}

	value, err := parseScalar(text, typeID)
	if err != nil {
		return nil, err
	}
	return ua.NewVariant(value)
}

// splitArray splits a comma-separated list, honouring double quotes so a string
// element may contain a comma.
func splitArray(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	var (
		parts   []string
		current strings.Builder
		quoted  bool
	)
	for _, r := range text {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ',' && !quoted:
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	parts = append(parts, strings.TrimSpace(current.String()))
	return parts
}

func parseArrayValue(items []string, typeID ua.TypeID) (*ua.Variant, error) {
	// A variant array must be a typed Go slice, not []any, so the slice is
	// built by reflection from the element type's own zero value.
	sample, err := parseScalar("", typeID)
	if err != nil {
		return nil, err
	}
	slice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(sample)), 0, len(items))

	for _, item := range items {
		element, err := parseScalar(item, typeID)
		if err != nil {
			return nil, err
		}
		slice = reflect.Append(slice, reflect.ValueOf(element))
	}

	return ua.NewVariant(slice.Interface())
}

// parseJSONValue accepts a JSON document, which is how an array of strings that
// contain commas, or a nested value, can be written unambiguously.
func parseJSONValue(text string, typeID ua.TypeID) (*ua.Variant, error) {
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, fmt.Errorf("invalid JSON value: %w", err)
	}

	if items, ok := decoded.([]any); ok {
		texts := make([]string, len(items))
		for i, item := range items {
			texts[i] = jsonScalarText(item)
		}
		return parseArrayValue(texts, typeID)
	}

	value, err := parseScalar(jsonScalarText(decoded), typeID)
	if err != nil {
		return nil, err
	}
	return ua.NewVariant(value)
}

func jsonScalarText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return formatFloat(typed, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}

// parseScalar converts one textual element into the Go type that gopcua encodes
// as the given built-in type. Empty text yields the type's zero value, which is
// also how parseArrayValue learns the element type.
func parseScalar(text string, typeID ua.TypeID) (any, error) {
	trimmed := strings.TrimSpace(text)

	integer := func(bits int) (int64, error) {
		if trimmed == "" {
			return 0, nil
		}
		return strconv.ParseInt(trimmed, 0, bits)
	}
	unsigned := func(bits int) (uint64, error) {
		if trimmed == "" {
			return 0, nil
		}
		return strconv.ParseUint(trimmed, 0, bits)
	}

	switch typeID {
	case ua.TypeIDBoolean:
		if trimmed == "" {
			return false, nil
		}
		return strconv.ParseBool(trimmed)

	case ua.TypeIDSByte:
		value, err := integer(8)
		return int8(value), err
	case ua.TypeIDByte:
		value, err := unsigned(8)
		return uint8(value), err
	case ua.TypeIDInt16:
		value, err := integer(16)
		return int16(value), err
	case ua.TypeIDUint16:
		value, err := unsigned(16)
		return uint16(value), err
	case ua.TypeIDInt32:
		value, err := integer(32)
		return int32(value), err
	case ua.TypeIDUint32:
		value, err := unsigned(32)
		return uint32(value), err
	case ua.TypeIDInt64:
		return integer(64)
	case ua.TypeIDUint64:
		return unsigned(64)

	case ua.TypeIDFloat:
		if trimmed == "" {
			return float32(0), nil
		}
		value, err := strconv.ParseFloat(trimmed, 32)
		return float32(value), err
	case ua.TypeIDDouble:
		if trimmed == "" {
			return float64(0), nil
		}
		return strconv.ParseFloat(trimmed, 64)

	case ua.TypeIDString:
		return text, nil
	case ua.TypeIDXMLElement:
		return ua.XMLElement(text), nil

	case ua.TypeIDDateTime:
		if trimmed == "" {
			return time.Time{}, nil
		}
		return ParseTime(trimmed, time.Now())

	case ua.TypeIDGUID:
		if trimmed == "" {
			return ua.NewGUID("00000000-0000-0000-0000-000000000000"), nil
		}
		guid := ua.NewGUID(trimmed)
		if guid == nil {
			return nil, fmt.Errorf("invalid GUID %q", text)
		}
		return guid, nil

	case ua.TypeIDByteString:
		if trimmed == "" {
			return []byte{}, nil
		}
		// Hex is the spelling a byte string is usually shown in, so accept it
		// first and fall back to treating the text as raw bytes.
		if decoded, err := hex.DecodeString(trimmed); err == nil {
			return decoded, nil
		}
		return []byte(text), nil

	case ua.TypeIDNodeID:
		if trimmed == "" {
			return ua.NewTwoByteNodeID(0), nil
		}
		return ParseNodeID(trimmed)

	case ua.TypeIDStatusCode:
		if trimmed == "" {
			return ua.StatusOK, nil
		}
		return parseStatusCode(trimmed)

	case ua.TypeIDQualifiedName:
		namespace, name := splitQualified(text)
		return &ua.QualifiedName{NamespaceIndex: namespace, Name: name}, nil

	case ua.TypeIDLocalizedText:
		return ua.NewLocalizedText(text), nil

	default:
		return nil, fmt.Errorf("cannot write values of type %s", TypeIDName(typeID))
	}
}

func parseStatusCode(text string) (ua.StatusCode, error) {
	if numeric, err := strconv.ParseUint(text, 0, 32); err == nil {
		return ua.StatusCode(numeric), nil
	}
	key := foldName(text)
	for status, description := range ua.StatusCodes {
		if foldName(strings.TrimPrefix(description.Name, "Status")) == key {
			return status, nil
		}
	}
	return 0, fmt.Errorf("unknown status code %q", text)
}

// splitQualified parses "2:Name" into its namespace index and name, treating a
// bare name as namespace 0.
func splitQualified(text string) (uint16, string) {
	index, name, found := strings.Cut(text, ":")
	if !found {
		return 0, text
	}
	namespace, err := strconv.ParseUint(index, 10, 16)
	if err != nil {
		return 0, text
	}
	return uint16(namespace), name
}

// ParseTime accepts the timestamp spellings a command line uses: RFC 3339, a
// bare date, a duration offset from now such as "-2h" or "-7d", or "now".
func ParseTime(text string, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(text)
	switch strings.ToLower(trimmed) {
	case "", "now":
		return now, nil
	}

	if offset, err := ParseDurationWithDays(trimmed); err == nil {
		return now.Add(offset), nil
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"15:04:05",
		"15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			// A time with no date means today, which is what a bare "14:30"
			// asks for.
			if layout == "15:04:05" || layout == "15:04" {
				return time.Date(now.Year(), now.Month(), now.Day(),
					parsed.Hour(), parsed.Minute(), parsed.Second(), 0, time.Local), nil
			}
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot read %q as a time: use RFC 3339, a date, or an offset such as -2h", text)
}

// ParseDurationWithDays extends Go's duration syntax with a day unit, since a
// history range is usually asked for in days.
func ParseDurationWithDays(text string) (time.Duration, error) {
	trimmed := strings.TrimSpace(text)
	negative := strings.HasPrefix(trimmed, "-")
	unsigned := strings.TrimPrefix(strings.TrimPrefix(trimmed, "-"), "+")

	if days, found := strings.CutSuffix(unsigned, "d"); found {
		count, err := strconv.ParseFloat(days, 64)
		if err != nil {
			return 0, err
		}
		duration := time.Duration(count * 24 * float64(time.Hour))
		if negative {
			return -duration, nil
		}
		return duration, nil
	}

	return time.ParseDuration(trimmed)
}

// AccessLevelText spells an access level bit mask as the flag names a person
// recognises, e.g. "CurrentRead|CurrentWrite".
func AccessLevelText(level ua.AccessLevelType) string {
	if level == ua.AccessLevelTypeNone {
		return "None"
	}

	flags := []struct {
		bit  ua.AccessLevelType
		name string
	}{
		{ua.AccessLevelTypeCurrentRead, "CurrentRead"},
		{ua.AccessLevelTypeCurrentWrite, "CurrentWrite"},
		{ua.AccessLevelTypeHistoryRead, "HistoryRead"},
		{ua.AccessLevelTypeHistoryWrite, "HistoryWrite"},
		{ua.AccessLevelTypeSemanticChange, "SemanticChange"},
		{ua.AccessLevelTypeStatusWrite, "StatusWrite"},
		{ua.AccessLevelTypeTimestampWrite, "TimestampWrite"},
	}

	var names []string
	for _, flag := range flags {
		if level&flag.bit != 0 {
			names = append(names, flag.name)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("0x%02X", uint8(level))
	}
	return strings.Join(names, "|")
}

// EventNotifierText spells the EventNotifier bit mask.
func EventNotifierText(mask byte) string {
	var names []string
	if mask&0x01 != 0 {
		names = append(names, "SubscribeToEvents")
	}
	if mask&0x04 != 0 {
		names = append(names, "HistoryRead")
	}
	if mask&0x08 != 0 {
		names = append(names, "HistoryWrite")
	}
	if len(names) == 0 {
		return "None"
	}
	return strings.Join(names, "|")
}

// ValueRankText explains the ValueRank attribute's negative sentinels, which are
// otherwise unreadable: -1 means a scalar, -2 means either, -3 means a scalar or
// a one-dimensional array.
func ValueRankText(rank int32) string {
	switch rank {
	case -3:
		return "ScalarOrOneDimension"
	case -2:
		return "Any"
	case -1:
		return "Scalar"
	case 0:
		return "OneOrMoreDimensions"
	case 1:
		return "OneDimension"
	default:
		return fmt.Sprintf("%dDimensions", rank)
	}
}
