package opc

import (
	"math"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
)

func TestStatusNameAndSeverity(t *testing.T) {
	for _, test := range []struct {
		status   ua.StatusCode
		name     string
		severity Severity
	}{
		// Zero is both StatusOK and StatusGood in the generated table; "Good" is
		// the name the specification uses for a status, and it is what the whole
		// tool compares against.
		{ua.StatusOK, "Good", SeverityGood},
		{ua.StatusBadNodeIDUnknown, "BadNodeIDUnknown", SeverityBad},
		{ua.StatusUncertainInitialValue, "UncertainInitialValue", SeverityUncertain},
	} {
		if got := StatusName(test.status); got != test.name {
			t.Errorf("StatusName(%#x) = %q, want %q", uint32(test.status), got, test.name)
		}
		if got := StatusSeverity(test.status); got != test.severity {
			t.Errorf("StatusSeverity(%q) = %v, want %v", test.name, got, test.severity)
		}
	}

	// An unknown code still has to render as something identifiable.
	if got := StatusName(ua.StatusCode(0x81234500)); got != "0x81234500" {
		t.Errorf("StatusName of an unknown code = %q, want hex", got)
	}
}

func TestTextRendersTheMeaningfulPartOfAValue(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{"double", 21.5, "21.5"},
		{"boolean", true, "true"},
		{"string", "Running", "Running"},
		{"int32", int32(-7), "-7"},
		{"array", []int32{1, 2, 3}, "[1, 2, 3]"},
		{"localized text", ua.NewLocalizedText("Kettle temperature"), "Kettle temperature"},
		{"qualified name in namespace 0", &ua.QualifiedName{Name: "Server"}, "Server"},
		{"qualified name elsewhere", &ua.QualifiedName{NamespaceIndex: 2, Name: "Tank"}, "2:Tank"},
		{"status code", ua.StatusBadTimeout, "BadTimeout"},
		{"printable byte string", []byte("abc"), "abc"},
		{"binary byte string", []byte{0x00, 0xff}, "00ff"},
	} {
		t.Run(test.name, func(t *testing.T) {
			variant, err := ua.NewVariant(test.value)
			if err != nil {
				t.Fatalf("NewVariant(%v): %v", test.value, err)
			}
			if got := Text(variant); got != test.want {
				t.Errorf("Text(%v) = %q, want %q", test.value, got, test.want)
			}
		})
	}

	if got := Text(nil); got != "" {
		t.Errorf("Text(nil) = %q, want an empty string", got)
	}
}

func TestJSONUsesNaturalJSONTypes(t *testing.T) {
	stamp := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	for _, test := range []struct {
		name  string
		value any
		want  any
	}{
		{"double stays a number", 21.5, 21.5},
		{"boolean stays a boolean", false, false},
		{"time becomes RFC 3339", stamp, "2024-03-01T12:00:00Z"},
		{"localized text becomes its text", ua.NewLocalizedText("hello"), "hello"},
		{"byte string becomes base64", []byte{0x01, 0x02}, "AQI="},
		{"status code becomes its name", ua.StatusBadTimeout, "BadTimeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			variant, err := ua.NewVariant(test.value)
			if err != nil {
				t.Fatalf("NewVariant: %v", err)
			}
			if got := JSON(variant); got != test.want {
				t.Errorf("JSON(%v) = %#v, want %#v", test.value, got, test.want)
			}
		})
	}

	variant, err := ua.NewVariant([]int32{4, 5})
	if err != nil {
		t.Fatalf("NewVariant: %v", err)
	}
	items, ok := JSON(variant).([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("JSON of an array = %#v, want a two-element slice", JSON(variant))
	}
}

// A non-finite float has no JSON spelling, and losing the whole result to an
// encoding error would be worse than reporting the value as text.
func TestJSONKeepsNonFiniteFloatsRepresentable(t *testing.T) {
	variant, err := ua.NewVariant(math.NaN())
	if err != nil {
		t.Fatalf("NewVariant: %v", err)
	}
	if got := JSON(variant); got != "NaN" {
		t.Errorf("JSON(NaN) = %#v, want \"NaN\"", got)
	}
}

func TestParseValueProducesTheDeclaredType(t *testing.T) {
	for _, test := range []struct {
		name   string
		text   string
		typeID ua.TypeID
		want   any
	}{
		{"boolean", "true", ua.TypeIDBoolean, true},
		{"int16", "-3", ua.TypeIDInt16, int16(-3)},
		{"uint32", "42", ua.TypeIDUint32, uint32(42)},
		{"double", "21.5", ua.TypeIDDouble, 21.5},
		{"float", "1.5", ua.TypeIDFloat, float32(1.5)},
		{"string", "hello world", ua.TypeIDString, "hello world"},
		{"hex byte string", "00ff", ua.TypeIDByteString, nil},
		{"int32 from hex", "0x10", ua.TypeIDInt32, int32(16)},
	} {
		t.Run(test.name, func(t *testing.T) {
			variant, err := ParseValue(test.text, test.typeID, false, false)
			if err != nil {
				t.Fatalf("ParseValue(%q, %v): %v", test.text, test.typeID, err)
			}
			if test.want == nil {
				return
			}
			if got := variant.Value(); got != test.want {
				t.Errorf("ParseValue(%q) = %#v, want %#v", test.text, got, test.want)
			}
		})
	}
}

func TestParseValueBuildsTypedArrays(t *testing.T) {
	variant, err := ParseValue("1,2,3", ua.TypeIDInt32, true, false)
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	values, ok := variant.Value().([]int32)
	if !ok {
		t.Fatalf("ParseValue produced %T, want []int32", variant.Value())
	}
	if len(values) != 3 || values[0] != 1 || values[2] != 3 {
		t.Errorf("ParseValue = %v, want [1 2 3]", values)
	}
}

// A quoted element may contain the separator, which is the only way to write a
// string array whose members contain commas.
func TestParseValueHonoursQuotesInAnArray(t *testing.T) {
	variant, err := ParseValue(`"a,b",c`, ua.TypeIDString, true, false)
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	values, ok := variant.Value().([]string)
	if !ok {
		t.Fatalf("ParseValue produced %T, want []string", variant.Value())
	}
	if len(values) != 2 || values[0] != "a,b" || values[1] != "c" {
		t.Errorf("ParseValue = %#v, want [\"a,b\" \"c\"]", values)
	}
}

func TestParseValueAcceptsJSON(t *testing.T) {
	variant, err := ParseValue(`[1, 2]`, ua.TypeIDInt32, false, true)
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	values, ok := variant.Value().([]int32)
	if !ok || len(values) != 2 {
		t.Fatalf("ParseValue produced %#v, want a two-element []int32", variant.Value())
	}

	if _, err := ParseValue("{not json", ua.TypeIDString, false, true); err == nil {
		t.Error("ParseValue accepted malformed JSON")
	}
}

func TestParseValueRejectsAnImpossibleValue(t *testing.T) {
	if _, err := ParseValue("banana", ua.TypeIDDouble, false, false); err == nil {
		t.Error("ParseValue accepted a non-numeric double")
	}
	if _, err := ParseValue("300", ua.TypeIDByte, false, false); err == nil {
		t.Error("ParseValue accepted a byte that does not fit")
	}
}

func TestParseTime(t *testing.T) {
	now := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)

	for _, test := range []struct {
		name string
		text string
		want time.Time
	}{
		{"now", "now", now},
		{"empty means now", "", now},
		{"offset", "-2h", now.Add(-2 * time.Hour)},
		{"offset in days", "-7d", now.Add(-7 * 24 * time.Hour)},
		{"RFC 3339", "2024-01-02T03:04:05Z", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := ParseTime(test.text, now)
			if err != nil {
				t.Fatalf("ParseTime(%q): %v", test.text, err)
			}
			if !parsed.Equal(test.want) {
				t.Errorf("ParseTime(%q) = %s, want %s", test.text, parsed, test.want)
			}
		})
	}

	// A bare date is local, so only the calendar date is asserted.
	parsed, err := ParseTime("2024-01-02", now)
	if err != nil {
		t.Fatalf("ParseTime: %v", err)
	}
	if parsed.Year() != 2024 || parsed.Month() != time.January || parsed.Day() != 2 {
		t.Errorf("ParseTime(2024-01-02) = %s, want that calendar date", parsed)
	}

	if _, err := ParseTime("half past nine", now); err == nil {
		t.Error("ParseTime accepted an unparseable time")
	}
}

func TestAccessLevelText(t *testing.T) {
	both := ua.AccessLevelTypeCurrentRead | ua.AccessLevelTypeCurrentWrite
	if got := AccessLevelText(both); got != "CurrentRead|CurrentWrite" {
		t.Errorf("AccessLevelText = %q, want CurrentRead|CurrentWrite", got)
	}
	if got := AccessLevelText(ua.AccessLevelTypeNone); got != "None" {
		t.Errorf("AccessLevelText(none) = %q, want None", got)
	}
}

func TestValueRankTextExplainsTheSentinels(t *testing.T) {
	for rank, want := range map[int32]string{
		-3: "ScalarOrOneDimension",
		-2: "Any",
		-1: "Scalar",
		0:  "OneOrMoreDimensions",
		1:  "OneDimension",
		2:  "2Dimensions",
	} {
		if got := ValueRankText(rank); got != want {
			t.Errorf("ValueRankText(%d) = %q, want %q", rank, got, want)
		}
	}
}

func TestNumericExtractsPlottableValues(t *testing.T) {
	for _, value := range []any{int32(3), uint16(4), 5.5, float32(6), true} {
		variant, err := ua.NewVariant(value)
		if err != nil {
			t.Fatalf("NewVariant(%v): %v", value, err)
		}
		if _, ok := Numeric(variant); !ok {
			t.Errorf("Numeric(%T) reported not numeric", value)
		}
	}

	variant, err := ua.NewVariant("text")
	if err != nil {
		t.Fatalf("NewVariant: %v", err)
	}
	if _, ok := Numeric(variant); ok {
		t.Error("Numeric reported a string as numeric")
	}
}
