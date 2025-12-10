package mcpgateway

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	storageClient     kclient.Client
	mcpSessionManager *mcp.SessionManager
	webhookHelper     *mcp.WebhookHelper
	jwks              system.EncodedJWKS
	tokenService      *persistent.TokenService
	baseURL           string
}

func NewHandler(storageClient kclient.Client, mcpSessionManager *mcp.SessionManager, webhookHelper *mcp.WebhookHelper, jwks system.EncodedJWKS, tokenService *persistent.TokenService, baseURL string) *Handler {
	return &Handler{
		storageClient:     storageClient,
		mcpSessionManager: mcpSessionManager,
		webhookHelper:     webhookHelper,
		jwks:              jwks,
		tokenService:      tokenService,
		baseURL:           baseURL,
	}
}

func (h *Handler) Proxy(req api.Context) error {
	if req.User.GetUID() == "anonymous" {
		req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_request", error_description="Invalid access token", resource_metadata="%s/.well-known/oauth-protected-resource%s"`, strings.TrimSuffix(req.APIBaseURL, "/api"), req.URL.Path))
		return apierrors.NewUnauthorized("user is not authenticated")
	}

	mcpURL, err := h.ensureServerIsDeployed(req)
	if err != nil {
		return fmt.Errorf("failed to ensure server is deployed: %v", err)
	}

	u, err := url.Parse(mcpURL)
	if err != nil {
		http.Error(req.ResponseWriter, err.Error(), http.StatusInternalServerError)
	}

	(&httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.Header.Set("X-Forwarded-Host", r.Host)
			scheme := "https"
			if strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1") {
				scheme = "http"
			}
			r.Header.Set("X-Forwarded-Proto", scheme)

			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

func (h *Handler) ensureServerIsDeployed(req api.Context) (string, error) {
	jwks, err := h.jwks(req.Context())
	if err != nil {
		return "", fmt.Errorf("failed to get jwks: %v", err)
	}

	mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(req, req.PathValue("mcp_id"), jwks)
	if err != nil {
		return "", fmt.Errorf("failed to get mcp server config: %w", err)
	}

	if mcpServer.Spec.Template {
		return "", apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}

	return h.mcpSessionManager.LaunchServer(req.Context(), mcpServerConfig)
}

func (h *Handler) SystemMCPServerConnect(req api.Context) error {
	id := req.PathValue("id")
	if !system.IsSystemMCPServerID(id) {
		return types.NewErrBadRequest("invalid system MCP server ID")
	}

	// Verify auth token has correct audience
	token := req.Request.Header.Get("Authorization")
	if token == "" {
		return types.NewErrHTTP(http.StatusUnauthorized, "missing authorization")
	}
	token = strings.TrimPrefix(token, "Bearer ")

	tokenCtx, err := h.tokenService.DecodeToken(req.Context(), token)
	if err != nil {
		return types.NewErrHTTP(http.StatusUnauthorized, "invalid token")
	}

	expectedAudience := fmt.Sprintf("%s/system-mcp-connect/%s", h.baseURL, id)
	if tokenCtx.Audience != expectedAudience {
		return types.NewErrHTTP(http.StatusForbidden, "token audience mismatch")
	}

	// Get the SystemMCPServer and convert to ServerConfig
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, id); err != nil {
		return types.NewErrNotFound("system MCP server not found")
	}

	// Get credentials for this system server
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{systemServer.Name}, systemServer.Name)
	credEnv := make(map[string]string)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to get credentials: %w", err)
		}
	} else {
		credEnv = cred.Env
	}

	// Transform to ServerConfig
	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, credEnv)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	// Launch the server and get its URL
	mcpURL, err := h.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	if err != nil {
		return fmt.Errorf("failed to launch system MCP server: %v", err)
	}

	// Proxy the MCP request to the SystemMCPServer
	u, err := url.Parse(mcpURL)
	if err != nil {
		return types.NewErrHTTP(http.StatusInternalServerError, err.Error())
	}

	(&httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.Header.Set("X-Forwarded-Host", r.Host)
			scheme := "https"
			if strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1") {
				scheme = "http"
			}
			r.Header.Set("X-Forwarded-Proto", scheme)

			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}
