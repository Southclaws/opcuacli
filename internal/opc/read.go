package opc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// ReadOptions controls a read of node attributes.
type ReadOptions struct {
	Nodes      []*ua.NodeID
	Attributes []ua.AttributeID
	IndexRange string
	MaxAge     time.Duration
	Timestamps ua.TimestampsToReturn
}

// Read reads every requested attribute of every requested node in one service
// call, and reports each result with its own status code: a node that cannot be
// read fails on its own rather than failing the command.
func Read(ctx context.Context, client Reader, options ReadOptions) ([]cligen.ReadResult, error) {
	if len(options.Attributes) == 0 {
		options.Attributes = []ua.AttributeID{ua.AttributeIDValue}
	}

	toRead := make([]*ua.ReadValueID, 0, len(options.Nodes)*len(options.Attributes))
	for _, node := range options.Nodes {
		for _, attribute := range options.Attributes {
			value := &ua.ReadValueID{NodeID: node, AttributeID: attribute}
			// An index range only means something for an array value; sending
			// one for another attribute earns BadIndexRangeNoData.
			if options.IndexRange != "" && attribute == ua.AttributeIDValue {
				value.IndexRange = options.IndexRange
			}
			toRead = append(toRead, value)
		}
	}
	if len(toRead) == 0 {
		return nil, nil
	}

	response, err := client.Read(ctx, &ua.ReadRequest{
		MaxAge:             float64(options.MaxAge / time.Millisecond),
		NodesToRead:        toRead,
		TimestampsToReturn: options.Timestamps,
	})
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if err := serviceResult(response.ResponseHeader); err != nil {
		return nil, err
	}

	results := make([]cligen.ReadResult, 0, len(toRead))
	for index, requested := range toRead {
		result := cligen.ReadResult{
			NodeID:    requested.NodeID.String(),
			Attribute: AttributeName(requested.AttributeID),
			Status:    StatusName(ua.StatusBadUnexpectedError),
		}
		if index < len(response.Results) {
			result = describeValue(result, response.Results[index], requested.AttributeID)
		}
		results = append(results, result)
	}

	return results, nil
}

// describeValue fills a read result from a data value, rendering the attribute
// the way its own definition means it: a bit mask as flag names, a data type as
// a type name, a value rank as its sentinel's meaning.
func describeValue(result cligen.ReadResult, value *ua.DataValue, attribute ua.AttributeID) cligen.ReadResult {
	if value == nil {
		return result
	}

	result.Status = StatusName(value.Status)
	if !value.SourceTimestamp.IsZero() {
		result.SourceTimestamp = &value.SourceTimestamp
	}
	if !value.ServerTimestamp.IsZero() {
		result.ServerTimestamp = &value.ServerTimestamp
	}
	if value.Status != ua.StatusOK || value.Value == nil {
		return result
	}

	result.Value = JSON(value.Value)
	if typeName := TypeName(value.Value); typeName != "" {
		result.Type = &typeName
	}

	text := AttributeText(attribute, value.Value)
	result.Text = &text

	if attribute == ua.AttributeIDArrayDimensions {
		result.ArrayDimensions = dimensions(value.Value)
	}

	return result
}

// AttributeText renders one attribute's value for a person: the flag names of a
// bit mask, the name behind a numeric enumeration, and the plain value for
// everything else.
func AttributeText(attribute ua.AttributeID, value *ua.Variant) string {
	if value == nil {
		return ""
	}

	switch attribute {
	case ua.AttributeIDNodeClass:
		return NodeClassName(ua.NodeClass(value.Int()))
	case ua.AttributeIDAccessLevel, ua.AttributeIDUserAccessLevel:
		return AccessLevelText(ua.AccessLevelType(value.Int()))
	case ua.AttributeIDAccessLevelEx:
		return fmt.Sprintf("0x%08X", uint32(value.Int()))
	case ua.AttributeIDEventNotifier:
		return EventNotifierText(byte(value.Int()))
	case ua.AttributeIDValueRank:
		return ValueRankText(int32(value.Int()))
	case ua.AttributeIDDataType:
		return dataTypeText(value)
	case ua.AttributeIDWriteMask, ua.AttributeIDUserWriteMask, ua.AttributeIDAccessRestrictions:
		return fmt.Sprintf("0x%08X", uint32(value.Int()))
	default:
		return Text(value)
	}
}

// dimensions extracts the ArrayDimensions attribute as a list of lengths.
func dimensions(value *ua.Variant) []int {
	raw, ok := value.Value().([]uint32)
	if !ok {
		return nil
	}
	sizes := make([]int, 0, len(raw))
	for _, size := range raw {
		sizes = append(sizes, int(size))
	}
	return sizes
}

// Attributes reads every attribute a node's class defines and returns the ones
// the server answered.
//
// Attributes that do not apply to a node come back as BadAttributeIdInvalid,
// which is normal rather than an error: a Variable has no Executable and an
// Object has no DataType. includeUnsupported keeps those rows, which is useful
// when a server is suspected of hiding an attribute it should have.
func Attributes(ctx context.Context, client Reader, node *ua.NodeID, includeUnsupported bool) (cligen.AttributeSet, error) {
	all := AllAttributes()

	results, err := Read(ctx, client, ReadOptions{
		Nodes:      []*ua.NodeID{node},
		Attributes: all,
		Timestamps: ua.TimestampsToReturnNeither,
	})
	if err != nil {
		return cligen.AttributeSet{}, err
	}

	set := cligen.AttributeSet{NodeID: node.String(), NodeClass: "Unspecified"}
	for index, result := range results {
		if index >= len(all) {
			break
		}
		attribute := all[index]

		supported := result.Status == StatusName(ua.StatusOK)
		if !supported && !includeUnsupported {
			continue
		}

		entry := cligen.Attribute{
			ID:     int(attribute),
			Name:   AttributeName(attribute),
			Status: result.Status,
			Value:  result.Value,
			Text:   result.Text,
			Type:   result.Type,
		}
		set.Attributes = append(set.Attributes, entry)

		if !supported {
			continue
		}
		switch attribute {
		case ua.AttributeIDNodeClass:
			if result.Text != nil {
				set.NodeClass = *result.Text
			}
		case ua.AttributeIDBrowseName:
			set.BrowseName = result.Text
		case ua.AttributeIDDisplayName:
			set.DisplayName = result.Text
		}
	}

	return set, nil
}

// ReadOne reads a single attribute and returns its raw variant, for the internal
// lookups that need a value rather than a rendered result.
func ReadOne(ctx context.Context, client Reader, node *ua.NodeID, attribute ua.AttributeID) (*ua.Variant, error) {
	response, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead:        []*ua.ReadValueID{{NodeID: node, AttributeID: attribute}},
		TimestampsToReturn: ua.TimestampsToReturnNeither,
	})
	if err != nil {
		return nil, err
	}
	if err := serviceResult(response.ResponseHeader); err != nil {
		return nil, err
	}
	if len(response.Results) == 0 || response.Results[0] == nil {
		return nil, fmt.Errorf("read %s %s: empty response", node, AttributeName(attribute))
	}

	result := response.Results[0]
	if result.Status != ua.StatusOK {
		return nil, fmt.Errorf("read %s %s: %s", node, AttributeName(attribute), StatusName(result.Status))
	}
	return result.Value, nil
}

// ReadMany reads one attribute of many nodes, returning the raw data values in
// request order. Per-node failures are left to the caller to inspect, which is
// what makes it usable for the optional parts of a server's information model.
func ReadMany(ctx context.Context, client Reader, nodes []*ua.NodeID, attribute ua.AttributeID) ([]*ua.DataValue, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	toRead := make([]*ua.ReadValueID, 0, len(nodes))
	for _, node := range nodes {
		toRead = append(toRead, &ua.ReadValueID{NodeID: node, AttributeID: attribute})
	}

	response, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead:        toRead,
		TimestampsToReturn: ua.TimestampsToReturnNeither,
	})
	if err != nil {
		return nil, err
	}
	if err := serviceResult(response.ResponseHeader); err != nil {
		return nil, err
	}
	return response.Results, nil
}

// ServiceError reports a service-level failure: one that applies to the whole
// request, unlike the per-item status codes inside a successful response. The
// status code is kept so a caller can recognise a particular fault - a service a
// server does not implement, most usefully - and say something better than the
// status name.
type ServiceError struct {
	Status ua.StatusCode
}

func (e *ServiceError) Error() string {
	// The specification's descriptions end in a full stop, and an error message
	// is punctuated by whatever prints it.
	if description := strings.TrimSuffix(StatusDescription(e.Status), "."); description != "" {
		return fmt.Sprintf("%s: %s", StatusName(e.Status), description)
	}
	return StatusName(e.Status)
}

// Unsupported reports whether the server said it does not implement the service.
func (e *ServiceError) Unsupported() bool {
	return e.Status == ua.StatusBadServiceUnsupported || e.Status == ua.StatusBadNotSupported ||
		e.Status == ua.StatusBadNotImplemented
}

// serviceResult turns a bad service-level status into a *ServiceError.
func serviceResult(header *ua.ResponseHeader) error {
	if header == nil || header.ServiceResult == ua.StatusOK {
		return nil
	}
	return &ServiceError{Status: header.ServiceResult}
}

// serviceError normalizes a failed service call into a *ServiceError.
//
// A fault reaches a caller by either of two routes: in the response header,
// which serviceResult handles, or as a bare status code returned by the client
// when it checks the header itself. Both become the same type here so that
// callers have one thing to test.
func serviceError(err error) error {
	if err == nil {
		return nil
	}

	var status ua.StatusCode
	if errors.As(err, &status) {
		return &ServiceError{Status: status}
	}

	var fault *ServiceError
	if errors.As(err, &fault) {
		return fault
	}
	return err
}

// Namespaces reads the server's namespace array, whose index positions are the
// `ns=` prefixes of every node id in the server.
func Namespaces(ctx context.Context, client Reader) ([]cligen.Namespace, error) {
	value, err := ReadOne(ctx, client, ua.NewNumericNodeID(0, id.Server_NamespaceArray), ua.AttributeIDValue)
	if err != nil {
		return nil, fmt.Errorf("read namespace array: %w", err)
	}

	uris, ok := value.Value().([]string)
	if !ok {
		return nil, fmt.Errorf("namespace array is %s, expected an array of strings", TypeName(value))
	}

	namespaces := make([]cligen.Namespace, 0, len(uris))
	for index, uri := range uris {
		namespaces = append(namespaces, cligen.Namespace{Index: index, URI: uri})
	}
	return namespaces, nil
}
