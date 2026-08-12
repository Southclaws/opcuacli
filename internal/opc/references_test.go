package opc

import (
	"context"
	"testing"

	"github.com/gopcua/opcua/ua"
)

// fakeSender answers a translate request with one target per path, recording what
// it was asked for.
type fakeSender struct {
	request *ua.TranslateBrowsePathsToNodeIDsRequest
	// fault, when set, is returned instead of a response, as the client does when
	// the server rejects the whole service.
	fault error
}

func (f *fakeSender) Send(_ context.Context, request ua.Request, handler func(ua.Response) error) error {
	if f.fault != nil {
		return f.fault
	}

	f.request = request.(*ua.TranslateBrowsePathsToNodeIDsRequest)

	results := make([]*ua.BrowsePathResult, 0, len(f.request.BrowsePaths))
	for index := range f.request.BrowsePaths {
		results = append(results, &ua.BrowsePathResult{
			StatusCode: ua.StatusOK,
			Targets: []*ua.BrowsePathTarget{{
				TargetID: &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(1, uint32(2000+index))},
			}},
		})
	}

	return handler(&ua.TranslateBrowsePathsToNodeIDsResponse{
		ResponseHeader: &ua.ResponseHeader{ServiceResult: ua.StatusOK},
		Results:        results,
	})
}

func TestResolveTranslatesEveryPathInOneCall(t *testing.T) {
	sender := &fakeSender{}

	resolved, err := Resolve(context.Background(), sender,
		ua.NewNumericNodeID(0, 85), []string{"Machine/Temp", "Machine/Press"}, 0)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved) != 2 {
		t.Fatalf("resolved %d path(s), want 2", len(resolved))
	}
	if resolved[0].NodeID == nil || *resolved[0].NodeID != "ns=1;i=2000" {
		t.Errorf("first path resolved to %v", resolved[0].NodeID)
	}
	if len(sender.request.BrowsePaths) != 2 {
		t.Errorf("sent %d path(s) in the request, want both in one call", len(sender.request.BrowsePaths))
	}
}

// A browse name can contain a colon - a namespace URI used as one does - so only
// a numeric prefix counts as a namespace index.
func TestResolveKeepsColonsInsideABrowseName(t *testing.T) {
	sender := &fakeSender{}

	_, err := Resolve(context.Background(), sender, ua.NewNumericNodeID(0, 85),
		[]string{"urn:southclaws:factory/COOK_KTL_01"}, 0)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	elements := sender.request.BrowsePaths[0].RelativePath.Elements
	if len(elements) != 2 {
		t.Fatalf("the path became %d element(s), want 2", len(elements))
	}
	if got := elements[0].TargetName.Name; got != "urn:southclaws:factory" {
		t.Errorf("the first element is %q, want the whole URI", got)
	}
	if got := elements[0].TargetName.NamespaceIndex; got != 0 {
		t.Errorf("the first element is in namespace %d, want 0", got)
	}
}

func TestResolveReadsANamespacePrefix(t *testing.T) {
	sender := &fakeSender{}

	_, err := Resolve(context.Background(), sender, ua.NewNumericNodeID(0, 85), []string{"2:Machine/3:Temp"}, 1)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	elements := sender.request.BrowsePaths[0].RelativePath.Elements
	if elements[0].TargetName.NamespaceIndex != 2 || elements[1].TargetName.NamespaceIndex != 3 {
		t.Errorf("namespaces are %d and %d, want 2 and 3",
			elements[0].TargetName.NamespaceIndex, elements[1].TargetName.NamespaceIndex)
	}

	// An element without a prefix falls back to the default namespace.
	sender = &fakeSender{}
	if _, err := Resolve(context.Background(), sender, ua.NewNumericNodeID(0, 85), []string{"Machine"}, 4); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := sender.request.BrowsePaths[0].RelativePath.Elements[0].TargetName.NamespaceIndex; got != 4 {
		t.Errorf("the element is in namespace %d, want the default of 4", got)
	}
}

func TestResolveRejectsAnEmptyPath(t *testing.T) {
	if _, err := Resolve(context.Background(), &fakeSender{}, ua.NewNumericNodeID(0, 85), []string{""}, 0); err == nil {
		t.Error("Resolve accepted an empty path")
	}
	if _, err := Resolve(context.Background(), &fakeSender{}, ua.NewNumericNodeID(0, 85), []string{"a//b"}, 0); err == nil {
		t.Error("Resolve accepted a path with an empty element")
	}
}

// A server that has not implemented the service returns a bare status code, and
// recognising it is what lets a command suggest crawling instead.
func TestResolveReportsAnUnsupportedServiceAsSuch(t *testing.T) {
	sender := &fakeSender{fault: ua.StatusBadServiceUnsupported}

	_, err := Resolve(context.Background(), sender, ua.NewNumericNodeID(0, 85), []string{"Machine"}, 0)
	if err == nil {
		t.Fatal("Resolve succeeded against a server that rejected the service")
	}

	fault, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("Resolve returned %T, want a *ServiceError", err)
	}
	if !fault.Unsupported() {
		t.Errorf("the fault %v is not reported as unsupported", fault)
	}
}

func TestReferencesListsBothDirections(t *testing.T) {
	inverse := reference("i=85", ua.NodeClassObject, "Objects")
	inverse.IsForward = false

	server := &fakeServer{
		children: map[string][]*ua.ReferenceDescription{
			"ns=1;s=Machine": {
				reference("ns=1;i=1", ua.NodeClassVariable, "Temperature"),
				inverse,
			},
		},
	}

	references, err := References(context.Background(), server, ReferenceOptions{
		Node:      ua.MustParseNodeID("ns=1;s=Machine"),
		Direction: ua.BrowseDirectionBoth,
	})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(references) != 2 {
		t.Fatalf("listed %d reference(s), want 2", len(references))
	}

	forward := 0
	for _, entry := range references {
		if entry.IsForward {
			forward++
		}
		if entry.ReferenceType != "HasComponent" {
			t.Errorf("reference type is %q, want the resolved name", entry.ReferenceType)
		}
	}
	if forward != 1 {
		t.Errorf("%d reference(s) are forward, want 1", forward)
	}
}

func TestReferencesFiltersByClass(t *testing.T) {
	server := &fakeServer{
		children: map[string][]*ua.ReferenceDescription{
			"ns=1;s=Machine": {
				reference("ns=1;i=1", ua.NodeClassVariable, "Temperature"),
				reference("ns=1;s=Restart", ua.NodeClassMethod, "Restart"),
			},
		},
	}

	references, err := References(context.Background(), server, ReferenceOptions{
		Node:      ua.MustParseNodeID("ns=1;s=Machine"),
		ClassMask: ua.NodeClassMethod,
	})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(references) != 1 || references[0].BrowseName != "Restart" {
		t.Errorf("listed %d reference(s), want just the method", len(references))
	}
}
