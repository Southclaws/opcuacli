package cligen

import (
	"time"
)

// One attribute of a node.
type Attribute struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// Human-readable rendering, including decoded bit flags.
	Text *string `json:"text,omitempty"`
	Type *string `json:"type,omitempty"`
	// The attribute value, as its natural JSON type.
	Value interface{} `json:"value,omitempty"`
}

// Every attribute of one node.
type AttributeSet struct {
	Attributes  []Attribute `json:"attributes"`
	BrowseName  *string     `json:"browseName,omitempty"`
	DisplayName *string     `json:"displayName,omitempty"`
	NodeClass   string      `json:"nodeClass"`
	NodeID      string      `json:"nodeId"`
}

// One input or output argument of a method.
type MethodArgument struct {
	DataType    string  `json:"dataType"`
	Description *string `json:"description,omitempty"`
	Name        string  `json:"name"`
	// The value passed or returned.
	Value     interface{} `json:"value,omitempty"`
	ValueRank *int        `json:"valueRank,omitempty"`
}

// The outcome of a method call.
type CallResult struct {
	// Per-argument status codes, when an argument was rejected.
	InputStatuses []string         `json:"inputStatuses,omitempty"`
	Inputs        []MethodArgument `json:"inputs,omitempty"`
	Method        string           `json:"method"`
	MethodName    *string          `json:"methodName,omitempty"`
	Object        string           `json:"object"`
	Ok            bool             `json:"ok"`
	Outputs       []MethodArgument `json:"outputs,omitempty"`
	Status        string           `json:"status"`
}

// Operational limits and conformance profiles of a server.
type Capabilities struct {
	Endpoint                     string   `json:"endpoint"`
	LocaleIds                    []string `json:"localeIds,omitempty"`
	MaxArrayLength               *int     `json:"maxArrayLength,omitempty"`
	MaxBrowseContinuationPoints  *int     `json:"maxBrowseContinuationPoints,omitempty"`
	MaxByteStringLength          *int     `json:"maxByteStringLength,omitempty"`
	MaxHistoryContinuationPoints *int     `json:"maxHistoryContinuationPoints,omitempty"`
	MaxMonitoredItemsPerCall     *int     `json:"maxMonitoredItemsPerCall,omitempty"`
	MaxNodesPerBrowse            *int     `json:"maxNodesPerBrowse,omitempty"`
	MaxNodesPerHistoryReadData   *int     `json:"maxNodesPerHistoryReadData,omitempty"`
	MaxNodesPerHistoryReadEvents *int     `json:"maxNodesPerHistoryReadEvents,omitempty"`
	MaxNodesPerHistoryUpdateData *int     `json:"maxNodesPerHistoryUpdateData,omitempty"`
	MaxNodesPerMethodCall        *int     `json:"maxNodesPerMethodCall,omitempty"`
	MaxNodesPerNodeManagement    *int     `json:"maxNodesPerNodeManagement,omitempty"`
	MaxNodesPerRead              *int     `json:"maxNodesPerRead,omitempty"`
	MaxNodesPerRegisterNodes     *int     `json:"maxNodesPerRegisterNodes,omitempty"`
	MaxNodesPerTranslate         *int     `json:"maxNodesPerTranslate,omitempty"`
	MaxNodesPerWrite             *int     `json:"maxNodesPerWrite,omitempty"`
	MaxQueryContinuationPoints   *int     `json:"maxQueryContinuationPoints,omitempty"`
	MaxStringLength              *int     `json:"maxStringLength,omitempty"`
	MinSupportedSampleRate       *float64 `json:"minSupportedSampleRate,omitempty"`
	ServerProfiles               []string `json:"serverProfiles,omitempty"`
	SoftwareCertificates         []string `json:"softwareCertificates,omitempty"`
}

// Details of an X.509 certificate.
type CertificateInfo struct {
	// The URI an OPC UA application must carry in a SAN.
	ApplicationURI   *string   `json:"applicationUri,omitempty"`
	DNSNames         []string  `json:"dnsNames,omitempty"`
	Expired          *bool     `json:"expired,omitempty"`
	ExtendedKeyUsage []string  `json:"extendedKeyUsage,omitempty"`
	IPAddresses      []string  `json:"ipAddresses,omitempty"`
	Issuer           string    `json:"issuer"`
	KeyBits          *int      `json:"keyBits,omitempty"`
	KeyUsage         []string  `json:"keyUsage,omitempty"`
	NotAfter         time.Time `json:"notAfter"`
	NotBefore        time.Time `json:"notBefore"`
	// File the certificate was read from, when local.
	Path               *string `json:"path,omitempty"`
	PublicKeyAlgorithm *string `json:"publicKeyAlgorithm,omitempty"`
	SelfSigned         *bool   `json:"selfSigned,omitempty"`
	SerialNumber       *string `json:"serialNumber,omitempty"`
	SignatureAlgorithm *string `json:"signatureAlgorithm,omitempty"`
	Subject            string  `json:"subject"`
	ThumbprintSha1     *string `json:"thumbprintSha1,omitempty"`
	ThumbprintSha256   *string `json:"thumbprintSha256,omitempty"`
}

// Diagnostic counters for a server.
type Diagnostics struct {
	// Whether the server has diagnostics collection switched on.
	Enabled                       bool   `json:"enabled"`
	Endpoint                      string `json:"endpoint"`
	RejectedRequestsCount         *int   `json:"rejectedRequestsCount,omitempty"`
	SecurityRejectedRequestsCount *int   `json:"securityRejectedRequestsCount,omitempty"`
	SessionCount                  *int   `json:"sessionCount,omitempty"`
	// Per-session counters, when requested.
	Sessions          []map[string]interface{} `json:"sessions,omitempty"`
	SubscriptionCount *int                     `json:"subscriptionCount,omitempty"`
	// The ServerDiagnosticsSummary counters.
	Summary map[string]interface{} `json:"summary,omitempty"`
}

// One endpoint an OPC UA server offers.
type Endpoint struct {
	ApplicationName *string          `json:"applicationName,omitempty"`
	ApplicationType *string          `json:"applicationType,omitempty"`
	ApplicationURI  *string          `json:"applicationUri,omitempty"`
	Certificate     *CertificateInfo `json:"certificate,omitempty"`
	DiscoveryUrls   []string         `json:"discoveryUrls,omitempty"`
	// URL a client connects to for this endpoint.
	EndpointURL      string  `json:"endpointUrl"`
	GatewayServerURI *string `json:"gatewayServerUri,omitempty"`
	ProductURI       *string `json:"productUri,omitempty"`
	// The server's own ranking of this endpoint.
	SecurityLevel int `json:"securityLevel"`
	// Message security mode, e.g. SignAndEncrypt.
	SecurityMode string `json:"securityMode"`
	// Security policy short name, e.g. Basic256Sha256.
	SecurityPolicy string `json:"securityPolicy"`
	// Full security policy URI.
	SecurityPolicyURI   *string `json:"securityPolicyUri,omitempty"`
	TransportProfileURI *string `json:"transportProfileUri,omitempty"`
	// User token types this endpoint accepts.
	UserTokenTypes []string `json:"userTokenTypes"`
}

type EndpointList = []Endpoint

// An event, read from history or received from a subscription.
type Event struct {
	EventID   *string `json:"eventId,omitempty"`
	EventType *string `json:"eventType,omitempty"`
	// Every selected field, keyed by its browse path.
	Fields      map[string]interface{} `json:"fields"`
	Message     *string                `json:"message,omitempty"`
	ReceiveTime *time.Time             `json:"receiveTime,omitempty"`
	Severity    *int                   `json:"severity,omitempty"`
	SourceName  *string                `json:"sourceName,omitempty"`
	Time        *time.Time             `json:"time,omitempty"`
}

type EventList = []Event

// One archived value.
type HistoryValue struct {
	NodeID          *string    `json:"nodeId,omitempty"`
	ServerTimestamp *time.Time `json:"serverTimestamp,omitempty"`
	SourceTimestamp *time.Time `json:"sourceTimestamp,omitempty"`
	Status          string     `json:"status"`
	Text            *string    `json:"text,omitempty"`
	// The archived value.
	Value interface{} `json:"value,omitempty"`
}

// Archived values of one node over a time range.
type HistoryResult struct {
	Count  int        `json:"count"`
	End    *time.Time `json:"end,omitempty"`
	NodeID string     `json:"nodeId"`
	Start  *time.Time `json:"start,omitempty"`
	// True when --limit stopped the read before the range ended.
	Truncated *bool          `json:"truncated,omitempty"`
	Values    []HistoryValue `json:"values,omitempty"`
}

// One entry of the server's namespace array.
type Namespace struct {
	Index int    `json:"index"`
	URI   string `json:"uri"`
}

type NamespaceList = []Namespace

// A server announced on the network via mDNS.
type NetworkServer struct {
	DiscoveryURL       string   `json:"discoveryUrl"`
	RecordID           int      `json:"recordId"`
	ServerCapabilities []string `json:"serverCapabilities,omitempty"`
	ServerName         string   `json:"serverName"`
}

type NetworkServerList = []NetworkServer

// A node found by browsing or searching the address space.
type Node struct {
	// Access level flags, e.g. CurrentRead|CurrentWrite.
	AccessLevel *string `json:"accessLevel,omitempty"`
	BrowseName  string  `json:"browseName"`
	// Nested nodes, when the walk descended.
	Children []Node `json:"children,omitempty"`
	// Data type name, for variables.
	DataType *string `json:"dataType,omitempty"`
	// Levels below the starting node.
	Depth       *int    `json:"depth,omitempty"`
	Description *string `json:"description,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	IsForward   *bool   `json:"isForward,omitempty"`
	Namespace   *int    `json:"namespace,omitempty"`
	// Object, Variable, Method, ObjectType, DataType, ...
	NodeClass string `json:"nodeClass"`
	// Node id in the canonical ns=;i= form.
	NodeID string `json:"nodeId"`
	// Slash-separated browse path from the starting node.
	Path *string `json:"path,omitempty"`
	// Reference that led to this node.
	ReferenceType  *string `json:"referenceType,omitempty"`
	TypeDefinition *string `json:"typeDefinition,omitempty"`
	// Current value, when values were requested.
	Value interface{} `json:"value,omitempty"`
	// Status code of the value read.
	ValueStatus *string `json:"valueStatus,omitempty"`
}

type NodeList = []Node

// Timings of one connectivity probe.
type PingResult struct {
	// Time to open the secure channel.
	Channel  *string `json:"channel,omitempty"`
	Endpoint string  `json:"endpoint"`
	Error    *string `json:"error,omitempty"`
	Ok       bool    `json:"ok"`
	// Time to read the server clock.
	Read           *string    `json:"read,omitempty"`
	SecurityMode   *string    `json:"securityMode,omitempty"`
	SecurityPolicy *string    `json:"securityPolicy,omitempty"`
	Sequence       int        `json:"sequence"`
	ServerTime     *time.Time `json:"serverTime,omitempty"`
	// Time to create and activate the session.
	Session *string `json:"session,omitempty"`
	Total   *string `json:"total,omitempty"`
}

type PingResultList = []PingResult

// A connection profile from the configuration file.
type Profile struct {
	AuthMode    *string `json:"authMode,omitempty"`
	Certificate *string `json:"certificate,omitempty"`
	Description *string `json:"description,omitempty"`
	// Endpoints in failover order.
	Endpoints      []string `json:"endpoints"`
	Insecure       *bool    `json:"insecure,omitempty"`
	IsDefault      bool     `json:"isDefault"`
	Name           string   `json:"name"`
	PrivateKey     *string  `json:"privateKey,omitempty"`
	RequestTimeout *string  `json:"requestTimeout,omitempty"`
	SecurityMode   *string  `json:"securityMode,omitempty"`
	SecurityPolicy *string  `json:"securityPolicy,omitempty"`
	SessionTimeout *string  `json:"sessionTimeout,omitempty"`
	Username       *string  `json:"username,omitempty"`
}

type ProfileList = []Profile

// One attribute read from one node.
type ReadResult struct {
	ArrayDimensions []int  `json:"arrayDimensions,omitempty"`
	Attribute       string `json:"attribute"`
	// Declared data type of the node.
	DataType        *string    `json:"dataType,omitempty"`
	NodeID          string     `json:"nodeId"`
	ServerTimestamp *time.Time `json:"serverTimestamp,omitempty"`
	SourceTimestamp *time.Time `json:"sourceTimestamp,omitempty"`
	// Status code of this read.
	Status string `json:"status"`
	// Human-readable rendering of the value.
	Text *string `json:"text,omitempty"`
	// Variant type of the value, e.g. Double.
	Type *string `json:"type,omitempty"`
	// The value, as its natural JSON type.
	Value interface{} `json:"value,omitempty"`
}

type ReadResultList = []ReadResult

// A server's redundancy support and failover set.
type Redundancy struct {
	// The failover order this client is configured with.
	ConfiguredEndpoints []string `json:"configuredEndpoints,omitempty"`
	Endpoint            string   `json:"endpoint"`
	// The other members of the redundant set.
	ServerUris   []string `json:"serverUris,omitempty"`
	ServiceLevel *int     `json:"serviceLevel,omitempty"`
	// None, Cold, Warm, Hot, Transparent, or HotAndMirrored.
	Support string `json:"support"`
	// True when failover is the server's job, not the client's.
	Transparent *bool `json:"transparent,omitempty"`
}

// One reference of a node.
type Reference struct {
	BrowseName  string  `json:"browseName"`
	DisplayName *string `json:"displayName,omitempty"`
	IsForward   bool    `json:"isForward"`
	NodeClass   string  `json:"nodeClass"`
	NodeID      string  `json:"nodeId"`
	// Browse name of the reference type.
	ReferenceType   string  `json:"referenceType"`
	ReferenceTypeID *string `json:"referenceTypeId,omitempty"`
	TypeDefinition  *string `json:"typeDefinition,omitempty"`
}

type ReferenceList = []Reference

// The outcome of translating one browse path.
type ResolvedPath struct {
	BrowseName *string `json:"browseName,omitempty"`
	NodeID     *string `json:"nodeId,omitempty"`
	Path       string  `json:"path"`
	// Status code of the translation.
	Status string `json:"status"`
}

type ResolvedPathList = []ResolvedPath

// An application description returned by FindServers.
type Server struct {
	ApplicationName     *string  `json:"applicationName,omitempty"`
	ApplicationType     string   `json:"applicationType"`
	ApplicationURI      string   `json:"applicationUri"`
	DiscoveryProfileURI *string  `json:"discoveryProfileUri,omitempty"`
	DiscoveryUrls       []string `json:"discoveryUrls,omitempty"`
	GatewayServerURI    *string  `json:"gatewayServerUri,omitempty"`
	ProductURI          *string  `json:"productUri,omitempty"`
}

// A server's status and build information.
type ServerInfo struct {
	BuildDate   *time.Time `json:"buildDate,omitempty"`
	BuildNumber *string    `json:"buildNumber,omitempty"`
	// Server clock minus local clock.
	ClockSkew           *string     `json:"clockSkew,omitempty"`
	CurrentTime         *time.Time  `json:"currentTime,omitempty"`
	Endpoint            string      `json:"endpoint"`
	ManufacturerName    *string     `json:"manufacturerName,omitempty"`
	Namespaces          []Namespace `json:"namespaces,omitempty"`
	ProductName         *string     `json:"productName,omitempty"`
	ProductURI          *string     `json:"productUri,omitempty"`
	SecondsTillShutdown *int        `json:"secondsTillShutdown,omitempty"`
	// 0 to 255; a redundant pair advertises the preferred member.
	ServiceLevel    *int       `json:"serviceLevel,omitempty"`
	ShutdownReason  *string    `json:"shutdownReason,omitempty"`
	SoftwareVersion *string    `json:"softwareVersion,omitempty"`
	StartTime       *time.Time `json:"startTime,omitempty"`
	// Running, Failed, Suspended, Shutdown, ...
	State  string  `json:"state"`
	Uptime *string `json:"uptime,omitempty"`
}

type ServerList = []Server

// One field of a structure or enumeration data type.
type TypeField struct {
	DataType        *string `json:"dataType,omitempty"`
	Description     *string `json:"description,omitempty"`
	IsOptional      *bool   `json:"isOptional,omitempty"`
	MaxStringLength *int    `json:"maxStringLength,omitempty"`
	Name            string  `json:"name"`
	// Numeric value, for an enumeration field.
	Value     *int `json:"value,omitempty"`
	ValueRank *int `json:"valueRank,omitempty"`
}

// A type node with its inheritance chain and fields.
type TypeInfo struct {
	BrowseName  string  `json:"browseName"`
	Description *string `json:"description,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	// Data type encodings the server publishes.
	Encodings  []string    `json:"encodings,omitempty"`
	Fields     []TypeField `json:"fields,omitempty"`
	IsAbstract *bool       `json:"isAbstract,omitempty"`
	// builtin, structure, enumeration, option-set, or abstract.
	Kind      string   `json:"kind"`
	NodeClass string   `json:"nodeClass"`
	NodeID    string   `json:"nodeId"`
	SubTypes  []string `json:"subTypes,omitempty"`
	// Inheritance chain, nearest ancestor first.
	SuperTypes []string `json:"superTypes,omitempty"`
}

// The declared type of a variable.
type TypeOf struct {
	ArrayDimensions []int   `json:"arrayDimensions,omitempty"`
	BuiltinType     *string `json:"builtinType,omitempty"`
	DataType        string  `json:"dataType"`
	DataTypeID      string  `json:"dataTypeId"`
	NodeID          string  `json:"nodeId"`
	// Scalar, Array, or a fixed dimension count.
	Rank       *string  `json:"rank,omitempty"`
	SuperTypes []string `json:"superTypes,omitempty"`
	ValueRank  *int     `json:"valueRank,omitempty"`
}

// The outcome of a write.
type WriteResult struct {
	Attribute string `json:"attribute"`
	DryRun    *bool  `json:"dryRun,omitempty"`
	NodeID    string `json:"nodeId"`
	Ok        bool   `json:"ok"`
	Status    string `json:"status"`
	// Variant type the value was written as.
	Type *string `json:"type,omitempty"`
	// The value that was written.
	Value interface{} `json:"value,omitempty"`
}
