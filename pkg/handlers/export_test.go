package handlers

// ImageURLUsesHTTPS exposes the unexported imageURLUsesHTTPS helper to external
// (handlers_test) tests so the InsertMedia transfer-protocol derivation can be
// exercised directly.
var ImageURLUsesHTTPS = imageURLUsesHTTPS

// ResolveOperatorImageURL exposes the unexported image-URL precedence helper so
// the per-deploy > per-BMC > global-default selection can be unit-tested
// directly, without driving the async deploy goroutine.
var ResolveOperatorImageURL = resolveOperatorImageURL
