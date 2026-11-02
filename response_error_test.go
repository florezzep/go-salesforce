package salesforce

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSalesforce_ResponseError(t *testing.T) {
	type account struct {
		Id string `soql:"selectColumn,fieldName=Id"`
	}
	type accountQuery struct {
		SelectClause account `soql:"selectClause,tableName=Account"`
	}
	methods := []struct {
		name string
		call func(*Salesforce) error
	}{
		{"Query", func(sf *Salesforce) error {
			return sf.Query("SELECT Id FROM Account", &[]account{})
		}},
		{"QueryStruct", func(sf *Salesforce) error {
			return sf.QueryStruct(accountQuery{}, &[]account{})
		}},
		{"DoRequest", func(sf *Salesforce) error {
			_, err := sf.DoRequest(http.MethodGet, "/query/", nil)
			return err
		}},
		{"DoApexRequest", func(sf *Salesforce) error {
			_, err := sf.DoApexRequest(http.MethodGet, "/example", nil)
			return err
		}},
	}
	responses := []struct {
		name   string
		status int
		body   string
	}{
		{
			"json_array",
			http.StatusInternalServerError,
			`[{"message":"Unavailable","errorCode":"SERVER_UNAVAILABLE"}]`,
		},
		{
			"json_object",
			http.StatusServiceUnavailable,
			`{"message":"Unavailable","errorCode":"SERVER_UNAVAILABLE"}`,
		},
		{
			"html_maintenance",
			http.StatusServiceUnavailable,
			"<html>Salesforce is down for maintenance</html>",
		},
		{"plain_text", http.StatusGatewayTimeout, "Gateway Timeout"},
		{"empty_body", http.StatusGatewayTimeout, ""},
		{
			"validation",
			http.StatusBadRequest,
			`[{"message":"Bad query","errorCode":"MALFORMED_QUERY"}]`,
		},
		{
			"authentication",
			http.StatusUnauthorized,
			`[{"message":"Invalid auth","errorCode":"INVALID_AUTH_HEADER"}]`,
		},
		{
			"request_limit",
			http.StatusForbidden,
			`[{"message":"Limit reached","errorCode":"REQUEST_LIMIT_EXCEEDED"}]`,
		},
		{
			"rate_limit",
			http.StatusTooManyRequests,
			`[{"message":"Too many requests","errorCode":"REQUEST_LIMIT_EXCEEDED"}]`,
		},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			for _, response := range responses {
				t.Run(response.name, func(t *testing.T) {
					calls := 0
					server := httptest.NewServer(
						http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls++
							w.WriteHeader(response.status)
							_, _ = io.WriteString(w, response.body)
						}),
					)
					defer server.Close()
					sf := buildSalesforceStruct(
						&authentication{InstanceUrl: server.URL, AccessToken: "test-token"},
					)

					err := method.call(sf)
					var responseErr *ResponseError
					if !errors.As(err, &responseErr) {
						t.Fatalf("error = %v, want *ResponseError", err)
					}
					if responseErr.StatusCode != response.status ||
						responseErr.Body != response.body {
						t.Errorf(
							"ResponseError = %#v, want status %d and body %q",
							responseErr,
							response.status,
							response.body,
						)
					}
					if err.Error() != response.body {
						t.Errorf("Error() = %q, want unchanged body %q", err.Error(), response.body)
					}
					if calls != 1 {
						t.Errorf("request count = %d, want 1", calls)
					}
				})
			}
		})
	}
}

func TestSalesforce_ResponseErrorAfterSessionRefresh(t *testing.T) {
	const invalidSession = `[{"message":"Session expired","errorCode":"INVALID_SESSION_ID"}]`
	tests := []struct {
		name          string
		refreshStatus int
		queryStatus   int
		queryBody     string
	}{
		{"success", http.StatusOK, http.StatusOK, `{"done":true,"records":[{"Id":"example"}]}`},
		{
			"service_unavailable",
			http.StatusOK,
			http.StatusServiceUnavailable,
			"Service Unavailable",
		},
		{"gateway_timeout", http.StatusOK, http.StatusGatewayTimeout, "Gateway Timeout"},
		{"session_still_invalid", http.StatusOK, http.StatusUnauthorized, invalidSession},
		{"refresh_failure", http.StatusServiceUnavailable, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, refreshes := 0, 0
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/services/oauth2/token" {
						refreshes++
						w.WriteHeader(tt.refreshStatus)
						_, _ = io.WriteString(w, `{"access_token":"refreshed-token"}`)
						return
					}
					queries++
					if queries == 1 {
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = io.WriteString(w, invalidSession)
						return
					}
					if r.Header.Get("Authorization") != "Bearer refreshed-token" {
						t.Error("retried query did not use the refreshed token")
					}
					w.WriteHeader(tt.queryStatus)
					_, _ = io.WriteString(w, tt.queryBody)
				}),
			)
			defer server.Close()
			sf := buildSalesforceStruct(&authentication{
				InstanceUrl: server.URL,
				AccessToken: "expired-token",
				grantType:   grantTypeClientCredentials,
			})

			err := sf.Query("SELECT Id FROM Account", &[]struct{ Id string }{})
			var responseErr *ResponseError
			if tt.refreshStatus != http.StatusOK {
				if err == nil || errors.As(err, &responseErr) {
					t.Fatalf("refresh error = %v, want original authentication error", err)
				}
				if err.Error() != "503 Service Unavailable: failed authentication" {
					t.Errorf("refresh error text changed: %v", err)
				}
				if queries != 1 || refreshes != 1 {
					t.Errorf("queries = %d, refreshes = %d, want 1 and 1", queries, refreshes)
				}
				return
			}
			if tt.queryStatus == http.StatusOK {
				if err != nil {
					t.Fatalf("query after refresh failed: %v", err)
				}
			} else {
				if !errors.As(err, &responseErr) {
					t.Fatalf("error = %v, want *ResponseError", err)
				}
				if responseErr.StatusCode != tt.queryStatus || responseErr.Body != tt.queryBody {
					t.Errorf("ResponseError = %#v, want status %d and body %q", responseErr, tt.queryStatus, tt.queryBody)
				}
			}
			if queries != 2 || refreshes != 1 {
				t.Errorf("queries = %d, refreshes = %d, want 2 and 1", queries, refreshes)
			}
		})
	}
}

func TestSalesforce_ResponseErrorOnLaterQueryPage(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = io.WriteString(
				w,
				`{"done":false,"records":[{"Id":"example"}],"nextRecordsUrl":"/services/data/v63.0/query/next-page"}`,
			)
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = io.WriteString(w, "Gateway Timeout")
	}))
	defer server.Close()
	sf := buildSalesforceStruct(&authentication{InstanceUrl: server.URL, AccessToken: "test-token"})

	err := sf.Query("SELECT Id FROM Account", &[]struct{ Id string }{})
	var responseErr *ResponseError
	if !errors.As(err, &responseErr) || responseErr.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("error = %v, want ResponseError with status 504", err)
	}
	if calls != 2 {
		t.Errorf("request count = %d, want 2", calls)
	}
}

func TestSalesforce_ResponseErrorDoesNotReplaceNetworkError(t *testing.T) {
	sf := buildSalesforceStruct(
		&authentication{InstanceUrl: "http://salesforce.test", AccessToken: "test-token"},
	)
	networkErr := errors.New("test network failure")
	sf.config.httpClient.Transport = responseErrorTransport(
		func(*http.Request) (*http.Response, error) {
			return nil, networkErr
		},
	)

	err := sf.Query("SELECT Id FROM Account", &[]struct{ Id string }{})
	var responseErr *ResponseError
	if !errors.Is(err, networkErr) || errors.As(err, &responseErr) {
		t.Fatalf("error = %v, want original network error without an HTTP status", err)
	}
}

func TestSalesforce_ResponseErrorDoesNotReplaceBodyReadError(t *testing.T) {
	sf := buildSalesforceStruct(
		&authentication{InstanceUrl: "http://salesforce.test", AccessToken: "test-token"},
	)
	readErr := errors.New("test body read failure")
	sf.config.httpClient.Transport = responseErrorTransport(
		func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     make(http.Header),
				Body: io.NopCloser(
					io.MultiReader(strings.NewReader("partial"), responseErrorReader{err: readErr}),
				),
			}, nil
		},
	)

	err := sf.Query("SELECT Id FROM Account", &[]struct{ Id string }{})
	var responseErr *ResponseError
	if !errors.Is(err, readErr) || errors.As(err, &responseErr) {
		t.Fatalf("error = %v, want original response-body read error", err)
	}
}

type responseErrorTransport func(*http.Request) (*http.Response, error)

func (f responseErrorTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type responseErrorReader struct {
	err error
}

func (r responseErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}
