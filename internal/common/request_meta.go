package common

import (
	"context"
	"net/http"
)

type RequestMetadata struct {
	UserID       uint   `json:"user_id"`
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	Role         string `json:"role"`
	Request      *http.Request
	ClientIP     string
	UserAgent    string
	RequestID    string
}

type metaCtxKey struct{}

func GetRequestMetadata(ctx context.Context) *RequestMetadata {
	v, _ := ctx.Value(metaCtxKey{}).(*RequestMetadata)
	return v
}

func SetRequestMetadata(ctx context.Context, meta *RequestMetadata) context.Context {
	return context.WithValue(ctx, metaCtxKey{}, meta)
}
