package utils

import (
	"context"
)

// TLSKey is the key related to tls.
type TLSKey string

const (
	// PouchTLSIssuer is the key of tls issue stored in context.
	PouchTLSIssuer TLSKey = "pouch.server.tls.issuer"
	// PouchTLSCommonName is the key of tls common name stored in context.
	PouchTLSCommonName TLSKey = "pouch.server.tls.cn"
)

// SetTLSIssuer set issuer name of tls to context.
func SetTLSIssuer(ctx context.Context, issuer string) context.Context {
	return context.WithValue(ctx, PouchTLSIssuer, issuer)
}

// GetTLSIssuer fetch issuer name from context.
func GetTLSIssuer(ctx context.Context) string {
	issuer := ctx.Value(PouchTLSIssuer)
	if issuer == nil {
		return ""
	}
	return issuer.(string)
}

// SetTLSCommonName set common name of tls to context.
func SetTLSCommonName(ctx context.Context, cn string) context.Context {
	return context.WithValue(ctx, PouchTLSCommonName, cn)
}

// GetTLSCommonName fetch common name from context.
func GetTLSCommonName(ctx context.Context) string {
	issuer := ctx.Value(PouchTLSCommonName)
	if issuer == nil {
		return ""
	}
	return issuer.(string)
}
