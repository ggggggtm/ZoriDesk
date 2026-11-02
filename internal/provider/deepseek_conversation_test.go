package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type deepSeekConversationDoer struct {
	request       *http.Request
	authorization string
	body          string
	model         string
	content       string
}

func (doer *deepSeekConversationDoer) Do(request *http.Request) (*http.Response, error) {
	doer.request = request
	doer.authorization = request.Header.Get("Authorization")
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	doer.body = string(body)
	model := doer.model
	if model == "" {
		model = DeepSeekConversationModelID
	}
	content := doer.content
	if content == "" {
		content = "Hello from DeepSeek"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"model":"` + model + `","choices":[{"message":{"role":"assistant","content":"` + content + `"}}]}`,
		)),
		Request: request,
	}, nil
}

func TestOpenAICompatibleConversationClientsUseFixedProviderContracts(t *testing.T) {
	tests := []struct {
		name             string
		providerID       string
		modelID          string
		endpoint         string
		newClient        func(OpenAICompatibleConversationConfig) (*DeepSeekConversationClient, error)
		wantTokenField   string
		forbidTokenField string
	}{
		{
			name: "Kimi", providerID: "kimi", modelID: KimiConversationModelID,
			endpoint: KimiConversationEndpoint, newClient: NewKimiConversationClient,
			wantTokenField: `"max_tokens":2048`, forbidTokenField: `"max_completion_tokens"`,
		},
		{
			name: "MiniMax", providerID: "minimax", modelID: MiniMaxConversationModelID,
			endpoint: MiniMaxConversationEndpoint, newClient: NewMiniMaxConversationClient,
			wantTokenField: `"max_completion_tokens":8192`, forbidTokenField: `"max_tokens"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doer := &deepSeekConversationDoer{
				model: test.modelID, content: "Hello from " + test.name,
			}
			client, err := test.newClient(OpenAICompatibleConversationConfig{
				Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Respond(
				context.Background(),
				[]ConversationMessage{{Role: "user", Content: "hello"}},
				[]byte("private-test-key"),
			)
			if err != nil {
				t.Fatal(err)
			}
			if response != "Hello from "+test.name || doer.request == nil ||
				doer.request.URL.String() != test.endpoint ||
				doer.authorization != "Bearer private-test-key" ||
				doer.request.Header.Get("Authorization") != "" ||
				!strings.Contains(doer.body, `"model":"`+test.modelID+`"`) ||
				!strings.Contains(doer.body, "provider="+test.providerID) ||
				!strings.Contains(doer.body, "model="+test.modelID) ||
				!strings.Contains(doer.body, "Do not infer or invent a different underlying model") ||
				strings.Contains(doer.body, "built on OpenAI") ||
				strings.Contains(doer.body, "GPT-series") ||
				!strings.Contains(doer.body, test.wantTokenField) ||
				strings.Contains(doer.body, test.forbidTokenField) ||
				strings.Contains(doer.body, "private-test-key") {
				t.Fatalf("response=%q request=%#v body=%q", response, doer.request, doer.body)
			}
		})
	}
}

func TestMiniMaxConversationSeparatesHiddenReasoningFromVisibleContent(t *testing.T) {
	doer := deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(string(body), `"reasoning_split":true`) {
			t.Fatalf("MiniMax request did not separate reasoning: %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"model":"MiniMax-M3","choices":[{"message":{"role":"assistant","reasoning_content":"private reasoning must not escape","content":"VISIBLE-ONLY"}}]}`,
			)),
			Request: request,
		}, nil
	})
	client, err := NewMiniMaxConversationClient(OpenAICompatibleConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("private-test-key"),
	)
	if err != nil || response != "VISIBLE-ONLY" || strings.Contains(response, "private reasoning") {
		t.Fatalf("response=%q error=%v", response, err)
	}
}

func TestMiniMaxConversationStripsInlineHiddenReasoningFromContent(t *testing.T) {
	doer := deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"model":"MiniMax-M3","choices":[{"message":{"role":"assistant","content":"<think>private reasoning must not escape</think>\nVISIBLE-ONLY"}}]}`,
			)),
			Request: request,
		}, nil
	})
	client, err := NewMiniMaxConversationClient(OpenAICompatibleConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("private-test-key"),
	)
	if err != nil || response != "VISIBLE-ONLY" || strings.Contains(response, "private reasoning") {
		t.Fatalf("response=%q error=%v", response, err)
	}
}

func TestMiniMaxConversationRejectsUnterminatedInlineHiddenReasoning(t *testing.T) {
	doer := deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"model":"MiniMax-M3","choices":[{"message":{"role":"assistant","content":"<think>private reasoning must not escape"}}]}`,
			)),
			Request: request,
		}, nil
	})
	client, err := NewMiniMaxConversationClient(OpenAICompatibleConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("private-test-key"),
	)
	if response != "" || err == nil {
		t.Fatalf("response=%q error=%v", response, err)
	}
}

func TestDeepSeekReasoningUsesExpandedBoundedCompletionBudget(t *testing.T) {
	doer := &deepSeekConversationDoer{}
	client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RespondConfigured(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("private-test-key"),
		"deepseek-v4-flash",
		"high",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doer.body, `"max_tokens":8192`) ||
		!strings.Contains(doer.body, `"reasoning_effort":"high"`) {
		t.Fatalf("reasoning request does not carry the bounded expanded budget: %s", doer.body)
	}
}

func TestDeepSeekConversationUsesFixedBoundedNonStreamingRequest(t *testing.T) {
	doer := &deepSeekConversationDoer{}
	client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("private-test-key")
	response, err := client.Respond(context.Background(), []ConversationMessage{
		{Role: "user", Content: "hello"},
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if response != "Hello from DeepSeek" || doer.request == nil ||
		doer.request.Method != http.MethodPost ||
		doer.request.URL.String() != "https://api.deepseek.com/chat/completions" ||
		doer.authorization != "Bearer private-test-key" ||
		!strings.Contains(doer.body, `"model":"deepseek-chat"`) ||
		!strings.Contains(doer.body, "provider=deepseek") ||
		!strings.Contains(doer.body, "model=deepseek-chat") ||
		!strings.Contains(doer.body, "Do not infer or invent a different underlying model") ||
		strings.Contains(doer.body, "built on OpenAI") ||
		strings.Contains(doer.body, "GPT-series") ||
		!strings.Contains(doer.body, `"stream":false`) ||
		strings.Contains(doer.body, "private-test-key") {
		t.Fatalf("response=%q request=%#v body=%q", response, doer.request, doer.body)
	}
}

func TestDeepSeekConversationKeepsLoomContextInSystemRole(t *testing.T) {
	doer := &deepSeekConversationDoer{}
	client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Respond(context.Background(), []ConversationMessage{
		{Role: "system", Content: `{"kind":"loom_role_context","items":[]}`},
		{Role: "user", Content: "SESSION-A"},
	}, []byte("private-test-key"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doer.body, `Loom-owned context capsule`) ||
		!strings.Contains(doer.body, `"content":"SESSION-A"`) ||
		strings.Contains(doer.body, `"role":"user","content":"{\"kind\":\"loom_role_context`) {
		t.Fatalf("context/user role boundary missing: %s", doer.body)
	}
}

func TestOpenAICompatibleConversationAcceptsProviderResolvedModelAlias(t *testing.T) {
	doer := &deepSeekConversationDoer{
		model: "deepseek-chat-2026-08-01", content: "resolved model reply",
	}
	client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("private-test-key"),
	)
	if err != nil || response != "resolved model reply" {
		t.Fatalf("response=%q error=%v", response, err)
	}
}

func TestOpenAICompatibleConversationClassifiesSafeInvalidResponseReason(t *testing.T) {
	client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"model":"deepseek-chat","choices":[{"message":{"role":"assistant","content":null}}]}`,
				)),
				Request: request,
			}, nil
		}),
		Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("private-test-key"),
	)
	failure, ok := ConversationFailureDetails(err)
	if !ok || failure.Code != "invalid_response" || failure.Stage != "provider_http" ||
		failure.HTTPStatus != http.StatusOK || failure.ProviderCode != "response_content" ||
		failure.Retryable {
		t.Fatalf("failure=%#v ok=%v error=%v", failure, ok, err)
	}
}

func TestOpenAICompatibleConversationClassifiesResponseBodyTimeout(t *testing.T) {
	client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(deepSeekConversationReaderFunc(func([]byte) (int, error) {
					return 0, context.DeadlineExceeded
				})),
				Request: request,
			}, nil
		}),
		Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("private-test-key"),
	)
	failure, ok := ConversationFailureDetails(err)
	if !ok || failure.Stage != "provider_http" || failure.Code != "timeout" ||
		failure.UserMessage != "Provider connection timed out." || !failure.Retryable {
		t.Fatalf("failure=%#v ok=%v error=%v", failure, ok, err)
	}
}

func TestDeepSeekConversationRejectsRedirectAndOversizedResponse(t *testing.T) {
	client, err := NewSystemDeepSeekConversationClient(5*time.Second, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if client == nil || client.endpoint != "https://api.deepseek.com/chat/completions" {
		t.Fatalf("client = %#v", client)
	}
	oversized := &deepSeekConversationDoer{}
	oversizedClient, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 4097))),
				Request:    request,
			}, nil
		}),
		Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oversizedClient.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: "hello"}},
		[]byte("key"),
	); err == nil {
		t.Fatal("oversized response accepted")
	}
	_ = oversized
}

func TestOpenAICompatibleConversationClassifiesSafeProviderFailures(t *testing.T) {
	tests := []struct {
		name              string
		status            int
		requestErr        error
		providerCode      string
		stage             string
		code              string
		userMessage       string
		retryable         bool
		retryAfterSeconds int64
	}{
		{name: "auth", status: http.StatusUnauthorized, providerCode: "invalid_api_key", stage: "provider_auth", code: "provider_auth", userMessage: "Provider rejected the selected account credential."},
		{name: "payment", status: http.StatusPaymentRequired, providerCode: "insufficient_balance", stage: "provider_http", code: "provider_insufficient_balance", userMessage: "Provider account has insufficient balance."},
		{name: "model", status: http.StatusNotFound, providerCode: "model_not_found", stage: "provider_http", code: "provider_model_unavailable", userMessage: "Selected model or Provider endpoint is unavailable."},
		{name: "invalid parameters", status: http.StatusUnprocessableEntity, providerCode: "invalid_parameter", stage: "provider_http", code: "provider_invalid_request", userMessage: "Provider rejected the request parameters."},
		{name: "rate limit", status: http.StatusTooManyRequests, providerCode: "rate_limit_exceeded", stage: "provider_rate_limit", code: "provider_rate_limit", userMessage: "Provider rate limit reached.", retryable: true, retryAfterSeconds: 18},
		{name: "server", status: http.StatusServiceUnavailable, providerCode: "server_error", stage: "provider_http", code: "provider_unavailable", userMessage: "Provider service is temporarily unavailable.", retryable: true},
		{name: "timeout", requestErr: context.DeadlineExceeded, stage: "provider_connect", code: "timeout", userMessage: "Provider connection timed out.", retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
				Client: deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
					if test.requestErr != nil {
						return nil, test.requestErr
					}
					response := &http.Response{
						StatusCode: test.status,
						Body: io.NopCloser(strings.NewReader(
							`{"error":{"code":"` + test.providerCode + `","message":"must never escape"}}`,
						)),
						Request: request,
						Header:  make(http.Header),
					}
					if test.retryAfterSeconds > 0 {
						response.Header.Set("Retry-After", "18")
					}
					return response, nil
				}),
				Timeout: time.Second, MaxResponseBytes: 4096,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Respond(
				context.Background(),
				[]ConversationMessage{{Role: "user", Content: "hello"}},
				[]byte("private-test-key"),
			)
			failure, ok := ConversationFailureDetails(err)
			if !ok || failure.Stage != test.stage || failure.Code != test.code ||
				failure.Retryable != test.retryable || failure.HTTPStatus != test.status ||
				failure.ProviderCode != test.providerCode ||
				failure.UserMessage != test.userMessage ||
				failure.RetryAfterSeconds != test.retryAfterSeconds ||
				!errors.Is(err, ErrOpenAICompatibleConversationUnavailable) ||
				strings.Contains(err.Error(), "must never escape") {
				t.Fatalf("failure=%#v ok=%v error=%v", failure, ok, err)
			}
		})
	}
}

func TestOpenAICompatibleConversationExcludesRequestCredentialFromProviderCode(t *testing.T) {
	for _, providerID := range []string{"deepseek", "kimi", "minimax"} {
		t.Run(providerID, func(t *testing.T) {
			testConversationProviderCodeCredentialBoundary(t, providerID)
		})
	}
}

// All credentials are synthetic. Responses derive echoes from the actual
// outgoing header, not from a separate sanitizer fixture or a real service.
func testConversationProviderCodeCredentialBoundary(t *testing.T, providerID string) {
	t.Helper()
	const fakeSecret = "fixture-credential-123"
	cases := []struct {
		name, body, secret, want string
	}{
		{name: "error code", body: `{"error":{"code":"ECHO"}}`},
		{name: "error type", body: `{"error":{"type":"ECHO"}}`},
		{name: "top code", body: `{"code":"ECHO"}`},
		{name: "base status", body: `{"base_resp":{"status_code":"ECHO"}}`},
		{name: "embedded", body: `{"error":{"code":"prefix-ECHO-suffix"}}`},
		{name: "escaped JSON", body: `{"error":{"code":"fixture-credential-\u0031\u0032\u0033"}}`},
		{name: "surrounding whitespace", body: `{"error":{"code":"  ECHO  "}}`},
		{name: "numeric credential", secret: "246810", body: `{"error":{"code":246810}}`},
		{name: "safe type fallback", body: `{"error":{"code":"ECHO","type":"authentication_error"}}`, want: "authentication_error"},
		{name: "safe top fallback", body: `{"error":{"code":"ECHO","type":"ECHO"},"code":"rate_limit_exceeded"}`, want: "rate_limit_exceeded"},
		{name: "safe base fallback", body: `{"error":{"code":"ECHO","type":"ECHO"},"code":"ECHO","base_resp":{"status_code":1008}}`, want: "1008"},
		{name: "all candidates echo", body: `{"error":{"code":"ECHO","type":"ECHO"},"code":"ECHO","base_resp":{"status_code":"ECHO"}}`},
		{name: "normal string", body: `{"error":{"code":"rate_limit_exceeded"}}`, want: "rate_limit_exceeded"},
		{name: "normal numeric", body: `{"base_resp":{"status_code":1008}}`, want: "1008"},
		{name: "normal numeric string", body: `{"code":"1008"}`, want: "1008"},
		{name: "unknown bounded code", body: `{"error":{"code":"Vendor.Future-Code_42"}}`, want: "Vendor.Future-Code_42"},
		{name: "not a key prefix blacklist", body: `{"code":"sk-vendor-status"}`, want: "sk-vendor-status"},
		{name: "unknown malformed", body: `{"error":{"code":"bad code / path"}}`},
		{name: "unknown object", body: `{"error":{"code":{"raw":"ECHO"}}}`},
		{name: "invalid JSON", body: `{"error":`},
		{name: "long code", body: `{"code":"` + strings.Repeat("x", 65) + `"}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			secret := test.secret
			if secret == "" {
				secret = fakeSecret
			}
			var observed *http.Request
			client := conversationCredentialBoundaryClient(t, providerID, deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
				observed = request
				echo := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
				if providerID == "anthropic" {
					echo = request.Header.Get("x-api-key")
				}
				if echo != secret {
					t.Fatal("request did not carry the synthetic credential")
				}
				return &http.Response{
					StatusCode: http.StatusTooManyRequests, Request: request,
					Header: http.Header{"Retry-After": []string{"18"}},
					Body:   io.NopCloser(strings.NewReader(strings.ReplaceAll(test.body, "ECHO", echo))),
				}, nil
			}))
			_, err := client.Respond(context.Background(), []ConversationMessage{{Role: "user", Content: "hello"}}, []byte(secret))
			failure, ok := ConversationFailureDetails(err)
			if !ok || failure.ProviderCode != test.want || failure.Stage != "provider_rate_limit" ||
				failure.Code != "provider_rate_limit" || failure.HTTPStatus != 429 ||
				!failure.Retryable || failure.RetryAfterSeconds != 18 ||
				failure.UserMessage != "Provider rate limit reached." ||
				!errors.Is(err, ErrProviderConversationUnavailable) {
				t.Fatal("credential boundary or original HTTP/retry classification not preserved")
			}
			encoded, marshalErr := json.Marshal(failure)
			if marshalErr != nil || strings.Contains(string(encoded), secret) || strings.Contains(err.Error(), secret) {
				t.Fatal("request credential escaped in failure details or error text")
			}
			if observed == nil || observed.Header.Get("Authorization") != "" || observed.Header.Get("x-api-key") != "" {
				t.Fatal("request credential header was retained")
			}
		})
	}
}

type conversationCredentialBoundaryResponder interface {
	Respond(context.Context, []ConversationMessage, []byte) (string, error)
}

func conversationCredentialBoundaryClient(t *testing.T, providerID string, doer HTTPDoer) conversationCredentialBoundaryResponder {
	t.Helper()
	config := OpenAICompatibleConversationConfig{Client: doer, Timeout: time.Second, MaxResponseBytes: 4096}
	var client conversationCredentialBoundaryResponder
	var err error
	switch providerID {
	case "deepseek":
		client, err = NewDeepSeekConversationClient(config)
	case "kimi":
		client, err = NewKimiConversationClient(config)
	case "minimax":
		client, err = NewMiniMaxConversationClient(config)
	case "anthropic":
		client, err = NewAnthropicConversationClient(AnthropicConversationConfig{Client: doer, Timeout: time.Second, MaxResponseBytes: 4096})
	default:
		t.Fatal("unknown test provider")
	}
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestOpenAICompatibleConversationProviderCodeCredentialCheckIsRequestScoped(t *testing.T) {
	for _, providerID := range []string{"deepseek", "kimi", "minimax", "anthropic"} {
		t.Run(providerID, func(t *testing.T) {
			const echo = "fixture-first-credential"
			client := conversationCredentialBoundaryClient(t, providerID, deepSeekConversationDoerFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusUnauthorized, Request: request,
					Body: io.NopCloser(strings.NewReader(`{"error":{"code":"` + echo + `"}}`))}, nil
			}))
			for index, secret := range []string{echo, "fixture-other-credential", echo} {
				_, err := client.Respond(context.Background(), []ConversationMessage{{Role: "user", Content: "hello"}}, []byte(secret))
				failure, ok := ConversationFailureDetails(err)
				want := ""
				if index == 1 {
					want = echo // Not this request's credential; no global mutable blacklist.
				}
				if !ok || failure.ProviderCode != want || failure.Retryable || failure.Code != "provider_auth" {
					t.Fatal("request-scoped credential filtering or auth classification changed")
				}
			}
		})
	}
}

type deepSeekConversationDoerFunc func(*http.Request) (*http.Response, error)

func (do deepSeekConversationDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

type deepSeekConversationReaderFunc func([]byte) (int, error)

func (read deepSeekConversationReaderFunc) Read(buffer []byte) (int, error) {
	return read(buffer)
}

// TestOpenAICompatibleConversationAcceptsContextCapsuleSizedMessage guards the
// regression where the Loom context capsule dispatch payload (allowed up to
// 32 KiB by the `context:loom-native:v1` adapter) grows with conversation
// history and exceeded the old 4096-byte per-message cap, making every later
// turn on a continuing thread fail with an opaque "conversation unavailable".
func TestOpenAICompatibleConversationAcceptsContextCapsuleSizedMessage(t *testing.T) {
	doer := &deepSeekConversationDoer{model: DeepSeekConversationModelID, content: "ok"}
	client, err := NewDeepSeekConversationClient(DeepSeekConversationConfig{
		Client: doer, Timeout: time.Second, MaxResponseBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	capsule := strings.Repeat(`{"item":"content","payload":"x"}`, 700)
	if len(capsule) <= 4096 || len(capsule) > 32<<10 {
		t.Fatalf("capsule fixture size %d must be >4096 and <=32KiB", len(capsule))
	}
	if _, err := client.Respond(
		context.Background(),
		[]ConversationMessage{{Role: "user", Content: capsule}},
		[]byte("private-test-key"),
	); err != nil {
		t.Fatalf("context-capsule-sized message rejected: %v", err)
	}
	if doer.request == nil || !strings.Contains(doer.body, "provider=deepseek") {
		t.Fatalf("request not dispatched: %#v", doer.request)
	}
}
