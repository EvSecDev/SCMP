package api

// ======================   FOR ALL ENDPOINTS   ======================

const (
	// Standard JSON-RPC 2.0 error codes
	rpcParseErr       = -32700 // Invalid JSON was received by the server.
	rpcInvalidRequest = -32600 // The JSON sent is not a valid Request object.
	rpcMethodNotFound = -32601 // The method does not exist / is not available.
	rpcInvalidParams  = -32602 // Invalid method parameter(s).
	rpcInternalError  = -32603 // Internal JSON-RPC error.

	// Custom error codes
	rpcUnauthorized = -32001 // Auth error
	//	rpcForbidden         = -32002 // Authenticated but not allowed
	rpcConflict = -32003 // Conflict, e.g., version mismatch
	//	rpcTimeout           = -32004 // Backend timeout
	//	rpcDependencyFailure = -32005 // Failed due to external system
	//	rpcNotImplemented    = -32006 // Method not implemented
	//	rpcRateLimited       = -32007 // Too many requests
	//	rpcDuplicateRequest  = -32008 // Same request ID or content
	//	rpcPartialFailure    = -32009 // Some parts failed
	//	rpcNoOp              = -32010 // Request succeeded but did nothing
	//	rpcDataCorruption    = -32011 // Storage or data integrity problem
	rpcInvalidState = -32012 // out-of-sync, illegal operation
// rpcValidationFailed  = -32013 // Input fails validation
// rpcMissingResource   = -32014 // file not found
// rpcQuotaExceeded     = -32015 // Storage, bandwidth, or usage quota hit
)
