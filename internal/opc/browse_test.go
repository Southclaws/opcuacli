package opc

import (
	"context"
	"testing"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// fakeServer answers Browse, BrowseNext and Read from a fixed address space, so
// the walk can be tested without a server.
type fakeServer struct {
	// children maps a node id to the references it answers with.
	children map[string][]*ua.ReferenceDescription
	// values maps a node id to the value it reads back as.
	values map[string]any
	// pages, when set for a node, is answered one page at a time with a
	// continuation point, in order.
	pages map[string][][]*ua.ReferenceDescription
	// pageState tracks which page a continuation point refers to.
	pageState map[string]int

	browseCalls int
	nextCalls   int
	readCalls   int
}

func reference(nodeID string, class ua.NodeClass, name string) *ua.ReferenceDescription {
	parsed := ua.MustParseNodeID(nodeID)
	return &ua.ReferenceDescription{
		ReferenceTypeID: ua.NewNumericNodeID(0, id.HasComponent),
		IsForward:       true,
		NodeID:          &ua.ExpandedNodeID{NodeID: parsed},
		BrowseName:      &ua.QualifiedName{Name: name},
		DisplayName:     ua.NewLocalizedText(name),
		NodeClass:       class,
	}
}

func (f *fakeServer) Browse(_ context.Context, request *ua.BrowseRequest) (*ua.BrowseResponse, error) {
	f.browseCalls++

	target := request.NodesToBrowse[0].NodeID.String()

	if pages, ok := f.pages[target]; ok && len(pages) > 0 {
		if f.pageState == nil {
			f.pageState = map[string]int{}
		}
		f.pageState[target] = 1
		return &ua.BrowseResponse{
			ResponseHeader: &ua.ResponseHeader{ServiceResult: ua.StatusOK},
			Results: []*ua.BrowseResult{{
				StatusCode:        ua.StatusOK,
				References:        pages[0],
				ContinuationPoint: []byte(target),
			}},
		}, nil
	}

	return &ua.BrowseResponse{
		ResponseHeader: &ua.ResponseHeader{ServiceResult: ua.StatusOK},
		Results: []*ua.BrowseResult{{
			StatusCode: ua.StatusOK,
			References: f.children[target],
		}},
	}, nil
}

func (f *fakeServer) BrowseNext(_ context.Context, request *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
	f.nextCalls++

	target := string(request.ContinuationPoints[0])
	pages := f.pages[target]
	index := f.pageState[target]

	if index >= len(pages) {
		return &ua.BrowseNextResponse{
			ResponseHeader: &ua.ResponseHeader{ServiceResult: ua.StatusOK},
			Results:        []*ua.BrowseResult{{StatusCode: ua.StatusOK}},
		}, nil
	}
	f.pageState[target] = index + 1

	continuation := []byte(target)
	if index+1 >= len(pages) {
		continuation = nil
	}
	return &ua.BrowseNextResponse{
		ResponseHeader: &ua.ResponseHeader{ServiceResult: ua.StatusOK},
		Results: []*ua.BrowseResult{{
			StatusCode:        ua.StatusOK,
			References:        pages[index],
			ContinuationPoint: continuation,
		}},
	}, nil
}

func (f *fakeServer) Read(_ context.Context, request *ua.ReadRequest) (*ua.ReadResponse, error) {
	f.readCalls++

	results := make([]*ua.DataValue, 0, len(request.NodesToRead))
	for _, requested := range request.NodesToRead {
		value, ok := f.values[requested.NodeID.String()]
		if !ok || requested.AttributeID != ua.AttributeIDValue {
			results = append(results, &ua.DataValue{Status: ua.StatusBadAttributeIDInvalid})
			continue
		}
		results = append(results, &ua.DataValue{Status: ua.StatusOK, Value: ua.MustVariant(value)})
	}

	return &ua.ReadResponse{
		ResponseHeader: &ua.ResponseHeader{ServiceResult: ua.StatusOK},
		Results:        results,
	}, nil
}

// plant is a small address space: a folder holding two variables and a method,
// with one nested folder.
func plant() *fakeServer {
	return &fakeServer{
		children: map[string][]*ua.ReferenceDescription{
			"i=85": {reference("ns=1;s=Machine", ua.NodeClassObject, "Machine")},
			"ns=1;s=Machine": {
				reference("ns=1;i=1", ua.NodeClassVariable, "Temperature"),
				reference("ns=1;i=2", ua.NodeClassVariable, "Pressure"),
				reference("ns=1;s=Restart", ua.NodeClassMethod, "Restart"),
				reference("ns=1;s=Sensors", ua.NodeClassObject, "Sensors"),
			},
			"ns=1;s=Sensors": {
				reference("ns=1;i=3", ua.NodeClassVariable, "Sensor01"),
			},
		},
		values: map[string]any{
			"ns=1;i=1": 21.5,
			"ns=1;i=2": 1.2,
			"ns=1;i=3": 7.0,
		},
	}
}

func root() *ua.NodeID { return ua.MustParseNodeID("i=85") }

func TestBrowseDepthLimitsTheWalk(t *testing.T) {
	server := plant()

	nodes, err := Browse(context.Background(), server, BrowseOptions{Root: root(), Depth: 1})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(nodes) != 1 || nodes[0].BrowseName != "Machine" {
		t.Fatalf("depth 1 returned %d node(s), want just Machine", len(nodes))
	}

	nodes, err = Browse(context.Background(), plant(), BrowseOptions{Root: root(), Depth: 2})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(nodes) != 5 {
		t.Fatalf("depth 2 returned %d node(s), want 5", len(nodes))
	}

	nodes, err = Browse(context.Background(), plant(), BrowseOptions{Root: root(), Depth: 0})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(nodes) != 6 {
		t.Fatalf("an unlimited walk returned %d node(s), want 6", len(nodes))
	}
}

// A flat walk reports a parent before its children, so a listing reads in the
// order the address space is laid out.
func TestBrowseFlatOrdersParentsBeforeChildren(t *testing.T) {
	nodes, err := Browse(context.Background(), plant(), BrowseOptions{Root: root(), Depth: 0})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}

	positions := map[string]int{}
	for index, node := range nodes {
		positions[node.BrowseName] = index
	}
	if positions["Machine"] > positions["Temperature"] {
		t.Error("Machine is reported after its child Temperature")
	}
	if positions["Sensors"] > positions["Sensor01"] {
		t.Error("Sensors is reported after its child Sensor01")
	}
}

func TestBrowseNestedBuildsATree(t *testing.T) {
	nodes, err := Browse(context.Background(), plant(), BrowseOptions{Root: root(), Depth: 0, Nested: true})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("a nested walk returned %d root(s), want 1", len(nodes))
	}

	machine := nodes[0]
	if len(machine.Children) != 4 {
		t.Fatalf("Machine has %d children, want 4", len(machine.Children))
	}
	for _, child := range machine.Children {
		if child.BrowseName == "Sensors" && len(child.Children) != 1 {
			t.Errorf("Sensors has %d children, want 1", len(child.Children))
		}
	}
}

// Descent passes through the classes being filtered out, so asking for only
// variables still finds the ones inside folders.
func TestBrowseClassFilterStillDescends(t *testing.T) {
	nodes, err := Browse(context.Background(), plant(), BrowseOptions{
		Root:      root(),
		Depth:     0,
		ClassMask: ua.NodeClassVariable,
	})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("returned %d node(s), want the 3 variables", len(nodes))
	}
	for _, node := range nodes {
		if node.NodeClass != "Variable" {
			t.Errorf("%s is a %s, want only variables", node.BrowseName, node.NodeClass)
		}
	}
}

func TestBrowseLimitStopsTheWalk(t *testing.T) {
	nodes, err := Browse(context.Background(), plant(), BrowseOptions{Root: root(), Depth: 0, Limit: 2})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("returned %d node(s), want 2", len(nodes))
	}
}

func TestBrowseReadsValuesInOneCallPerLevel(t *testing.T) {
	server := plant()

	nodes, err := Browse(context.Background(), server, BrowseOptions{Root: root(), Depth: 0, WithValues: true})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}

	found := map[string]any{}
	for _, node := range nodes {
		if node.Value != nil {
			found[node.BrowseName] = node.Value
		}
	}
	if found["Temperature"] != 21.5 {
		t.Errorf("Temperature read back as %#v, want 21.5", found["Temperature"])
	}
	if len(found) != 3 {
		t.Errorf("read %d value(s), want 3", len(found))
	}

	// Two levels hold variables, so two reads is the whole cost.
	if server.readCalls != 2 {
		t.Errorf("made %d read call(s), want 1 per level holding variables", server.readCalls)
	}
}

// Some servers page a browse with overlapping windows, repeating references
// already sent. A node listed twice reads as two nodes, so repeats are dropped.
func TestBrowseDedupesOverlappingPages(t *testing.T) {
	first := []*ua.ReferenceDescription{
		reference("ns=1;i=1", ua.NodeClassVariable, "A"),
		reference("ns=1;i=2", ua.NodeClassVariable, "B"),
	}
	overlapping := []*ua.ReferenceDescription{
		reference("ns=1;i=1", ua.NodeClassVariable, "A"),
		reference("ns=1;i=2", ua.NodeClassVariable, "B"),
		reference("ns=1;i=3", ua.NodeClassVariable, "C"),
	}

	server := &fakeServer{
		pages: map[string][][]*ua.ReferenceDescription{
			"i=85": {first, overlapping},
		},
	}

	nodes, err := Browse(context.Background(), server, BrowseOptions{Root: root(), Depth: 1})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}

	if len(nodes) != 3 {
		names := make([]string, 0, len(nodes))
		for _, node := range nodes {
			names = append(names, node.BrowseName)
		}
		t.Fatalf("returned %v, want each of A, B and C once", names)
	}
}

// A server whose continuation point never advances would otherwise be followed
// for ever.
func TestBrowseStopsWhenAPageAddsNothing(t *testing.T) {
	repeated := []*ua.ReferenceDescription{reference("ns=1;i=1", ua.NodeClassVariable, "A")}

	server := &fakeServer{
		pages: map[string][][]*ua.ReferenceDescription{
			"i=85": {repeated, repeated, repeated, repeated},
		},
	}

	nodes, err := Browse(context.Background(), server, BrowseOptions{Root: root(), Depth: 1})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("returned %d node(s), want 1", len(nodes))
	}
	if server.nextCalls > 1 {
		t.Errorf("followed %d continuation points, want to stop after the first repeat", server.nextCalls)
	}
}

// A reference graph can contain cycles, and a walk that revisits a node it has
// already descended into never finishes.
func TestBrowseSurvivesACycle(t *testing.T) {
	server := &fakeServer{
		children: map[string][]*ua.ReferenceDescription{
			"i=85":     {reference("ns=1;s=A", ua.NodeClassObject, "A")},
			"ns=1;s=A": {reference("ns=1;s=B", ua.NodeClassObject, "B")},
			"ns=1;s=B": {reference("ns=1;s=A", ua.NodeClassObject, "A")},
		},
	}

	nodes, err := Browse(context.Background(), server, BrowseOptions{Root: root(), Depth: 0})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("a cyclic address space returned nothing")
	}
}

func TestBrowseEmitsEachNodeAsItIsFound(t *testing.T) {
	var emitted []string
	_, err := Browse(context.Background(), plant(), BrowseOptions{
		Root:  root(),
		Depth: 0,
		Emit: func(node cligen.Node) error {
			emitted = append(emitted, node.BrowseName)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(emitted) != 6 {
		t.Errorf("emitted %d node(s), want 6", len(emitted))
	}
}

func TestBrowseReportsABadStatus(t *testing.T) {
	server := &failingServer{status: ua.StatusBadNodeIDUnknown}
	if _, err := Browse(context.Background(), server, BrowseOptions{Root: root(), Depth: 1}); err == nil {
		t.Fatal("Browse succeeded against a server that rejected the node")
	}
}

type failingServer struct {
	status ua.StatusCode
}

func (f *failingServer) Browse(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
	return &ua.BrowseResponse{
		ResponseHeader: &ua.ResponseHeader{ServiceResult: ua.StatusOK},
		Results:        []*ua.BrowseResult{{StatusCode: f.status}},
	}, nil
}

func (f *failingServer) BrowseNext(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
	return nil, nil
}

func (f *failingServer) Read(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
	return nil, nil
}

func TestMatchName(t *testing.T) {
	node := cligen.Node{
		NodeID:      "ns=2;i=7",
		BrowseName:  "COOK_KTL_01_TEMP_C",
		DisplayName: new("Kettle temperature"),
	}

	for _, test := range []struct {
		name    string
		pattern string
		field   string
		regex   bool
		want    bool
	}{
		{"substring is case insensitive", "temp", "any", false, true},
		{"browse name only", "kettle", "browse-name", false, false},
		{"display name", "kettle", "display-name", false, true},
		{"node id", "ns=2", "node-id", false, true},
		{"regular expression", `^COOK_.*_C$`, "browse-name", true, true},
		{"no match", "boiler", "any", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			match, err := MatchName(test.pattern, test.field, test.regex)
			if err != nil {
				t.Fatalf("MatchName: %v", err)
			}
			if got := match(node); got != test.want {
				t.Errorf("MatchName(%q, %q) = %v, want %v", test.pattern, test.field, got, test.want)
			}
		})
	}

	if match, err := MatchName("", "any", false); err != nil || match != nil {
		t.Error("an empty pattern should not filter at all")
	}
	if _, err := MatchName("([", "any", true); err == nil {
		t.Error("MatchName accepted a malformed regular expression")
	}
}

func TestMatchNamespaces(t *testing.T) {
	match, err := MatchNamespaces([]string{"1", " 2 "})
	if err != nil {
		t.Fatalf("MatchNamespaces: %v", err)
	}

	if !match(cligen.Node{Namespace: new(2)}) {
		t.Error("namespace 2 was rejected")
	}
	if match(cligen.Node{Namespace: new(3)}) {
		t.Error("namespace 3 was accepted")
	}
	if match(cligen.Node{}) {
		t.Error("a node with no namespace was accepted")
	}

	if _, err := MatchNamespaces([]string{"two"}); err == nil {
		t.Error("MatchNamespaces accepted a non-numeric index")
	}
}

func TestAllOfRequiresEveryFilter(t *testing.T) {
	yes := func(cligen.Node) bool { return true }
	no := func(cligen.Node) bool { return false }

	if AllOf() != nil {
		t.Error("AllOf with no filters should not filter")
	}
	if AllOf(nil, nil) != nil {
		t.Error("AllOf with only nil filters should not filter")
	}
	if !AllOf(yes, nil)(cligen.Node{}) {
		t.Error("AllOf(yes) rejected a node")
	}
	if AllOf(yes, no)(cligen.Node{}) {
		t.Error("AllOf(yes, no) accepted a node")
	}
}
