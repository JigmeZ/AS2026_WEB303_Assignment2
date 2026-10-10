package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// httpStatusFromGRPC maps gRPC status codes to HTTP status codes.
func httpStatusFromGRPC(c codes.Code) int {
	switch c {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest // 400
	case codes.NotFound:
		return http.StatusNotFound // 404
	case codes.AlreadyExists, codes.FailedPrecondition:
		return http.StatusConflict // 409
	case codes.Unavailable:
		return http.StatusServiceUnavailable // 503
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout // 504
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError // 500
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError converts a gRPC error into an HTTP JSON error response.
func writeError(w http.ResponseWriter, err error) {
	st := status.Convert(err)
	writeJSON(w, httpStatusFromGRPC(st.Code()), map[string]string{
		"error":     st.Message(),
		"grpc_code": st.Code().String(),
	})
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{
		"error":     msg,
		"grpc_code": codes.InvalidArgument.String(),
	})
}

// pathID parses the {id} path parameter.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, status.Error(codes.InvalidArgument, "id in URL must be a positive integer")
	}
	return id, nil
}

// decodeBody reads a JSON body (max 1 MB, unknown fields rejected).
func decodeBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
