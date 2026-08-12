package opc

import (
	"testing"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
)

func TestParseNodeID(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{"numeric shorthand", "2253", "i=2253"},
		{"standard form", "i=2253", "i=2253"},
		{"namespaced string", "ns=2;s=Machine/Temp", "ns=2;s=Machine/Temp"},
		{"well-known name", "Objects", "i=85"},
		{"well-known name is case insensitive", "server", "i=2253"},
		{"surrounding space", "  i=85  ", "i=85"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := ParseNodeID(test.input)
			if err != nil {
				t.Fatalf("ParseNodeID(%q): %v", test.input, err)
			}
			if got := parsed.String(); got != test.want {
				t.Errorf("ParseNodeID(%q) = %s, want %s", test.input, got, test.want)
			}
		})
	}
}

func TestParseNodeIDRejectsNonsense(t *testing.T) {
	for _, input := range []string{"", "   ", "ns=2", "ns=x;i=1"} {
		if _, err := ParseNodeID(input); err == nil {
			t.Errorf("ParseNodeID(%q) succeeded, want an error", input)
		}
	}
}

// A word with no prefix is a string identifier in namespace 0, which is what the
// specification's textual form means. It is worth pinning down because it makes a
// mistyped well-known name into a node id the server rejects rather than a
// parse error here.
func TestParseNodeIDTreatsABareWordAsAStringIdentifier(t *testing.T) {
	parsed, err := ParseNodeID("Temperatur")
	if err != nil {
		t.Fatalf("ParseNodeID: %v", err)
	}
	if got := parsed.String(); got != "s=Temperatur" {
		t.Errorf("ParseNodeID(%q) = %s, want s=Temperatur", "Temperatur", got)
	}
}

func TestParseAttributeAcceptsEverySpelling(t *testing.T) {
	for _, input := range []string{"AccessLevel", "accesslevel", "access-level", "access_level", "17"} {
		attribute, err := ParseAttribute(input)
		if err != nil {
			t.Fatalf("ParseAttribute(%q): %v", input, err)
		}
		if attribute != ua.AttributeIDAccessLevel {
			t.Errorf("ParseAttribute(%q) = %v, want AccessLevel", input, attribute)
		}
	}

	if _, err := ParseAttribute("NotAnAttribute"); err == nil {
		t.Error("ParseAttribute accepted an unknown name")
	}
	if _, err := ParseAttribute("999"); err == nil {
		t.Error("ParseAttribute accepted an unknown id")
	}
}

func TestAttributeNamesRoundTrip(t *testing.T) {
	for _, attribute := range AllAttributes() {
		name := AttributeName(attribute)
		if name == "" {
			t.Fatalf("attribute %d has no name", attribute)
		}
		parsed, err := ParseAttribute(name)
		if err != nil {
			t.Fatalf("ParseAttribute(%q): %v", name, err)
		}
		if parsed != attribute {
			t.Errorf("%q round-tripped to %v, want %v", name, parsed, attribute)
		}
	}
}

func TestParseNodeClassMask(t *testing.T) {
	for _, test := range []struct {
		name    string
		classes []string
		want    ua.NodeClass
	}{
		{"empty means everything", nil, ua.NodeClassAll},
		{"one class", []string{"variable"}, ua.NodeClassVariable},
		{"several classes combine", []string{"variable", "method"}, ua.NodeClassVariable | ua.NodeClassMethod},
		{"separators are ignored", []string{"object-type"}, ua.NodeClassObjectType},
		{"all is a shortcut", []string{"all"}, ua.NodeClassAll},
	} {
		t.Run(test.name, func(t *testing.T) {
			mask, err := ParseNodeClassMask(test.classes)
			if err != nil {
				t.Fatalf("ParseNodeClassMask(%v): %v", test.classes, err)
			}
			if mask != test.want {
				t.Errorf("ParseNodeClassMask(%v) = %v, want %v", test.classes, mask, test.want)
			}
		})
	}

	if _, err := ParseNodeClassMask([]string{"widget"}); err == nil {
		t.Error("ParseNodeClassMask accepted an unknown class")
	}
}

// A described node's class is filtered by comparing it to a mask, which only
// works if the name the describer produced parses back to the same class. The
// two directions of the gopcua mapping disagree on the prefix, so this guards
// the conversion that bridges them.
func TestNodeClassNameRoundTrip(t *testing.T) {
	for _, class := range []ua.NodeClass{
		ua.NodeClassObject,
		ua.NodeClassVariable,
		ua.NodeClassMethod,
		ua.NodeClassObjectType,
		ua.NodeClassVariableType,
		ua.NodeClassReferenceType,
		ua.NodeClassDataType,
		ua.NodeClassView,
	} {
		name := NodeClassName(class)
		if got := NodeClassFromName(name); got != class {
			t.Errorf("NodeClassFromName(NodeClassName(%v)) = %v, want %v", class, got, class)
		}
	}
}

func TestParseReferenceType(t *testing.T) {
	for _, test := range []struct {
		input string
		want  uint32
	}{
		{"", id.References},
		{"all", id.References},
		{"HierarchicalReferences", id.HierarchicalReferences},
		{"hierarchical-references", id.HierarchicalReferences},
		{"hascomponent", id.HasComponent},
		{"i=47", id.HasComponent},
	} {
		parsed, err := ParseReferenceType(test.input)
		if err != nil {
			t.Fatalf("ParseReferenceType(%q): %v", test.input, err)
		}
		if parsed.IntID() != test.want {
			t.Errorf("ParseReferenceType(%q) = %s, want i=%d", test.input, parsed, test.want)
		}
	}
}

func TestStandardNameOnlyNamesStandardNodes(t *testing.T) {
	if got := StandardName(ua.NewNumericNodeID(0, id.Server)); got != "Server" {
		t.Errorf("StandardName(i=2253) = %q, want Server", got)
	}
	if got := StandardName(ua.NewStringNodeID(2, "Machine")); got != "" {
		t.Errorf("StandardName(ns=2;s=Machine) = %q, want an empty string", got)
	}
	if got := StandardName(nil); got != "" {
		t.Errorf("StandardName(nil) = %q, want an empty string", got)
	}
}
