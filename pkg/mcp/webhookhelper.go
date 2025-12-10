package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/tools/cache"
)

type WebhookHelper struct {
	indexer          cache.Indexer
	defaultBaseImage string
}

func NewWebhookHelper(indexer cache.Indexer, defaultBaseImage string) *WebhookHelper {
	return &WebhookHelper{
		indexer:          indexer,
		defaultBaseImage: defaultBaseImage,
	}
}

type Webhook struct {
	Name, DisplayName  string
	URL, Secret, Image string
	Definitions        []string

	// New fields for SystemMCPServer hooks
	SystemMCPServerName string // If non-empty, this is a SystemMCPServer hook
	ToolName            string // Tool to call within the SystemMCPServer
}

// IsSystemMCPServerHook returns true if this webhook targets a SystemMCPServer tool
func (w Webhook) IsSystemMCPServerHook() bool {
	return w.SystemMCPServerName != ""
}

func (wh *WebhookHelper) GetWebhooksForMCPServer(ctx context.Context, gptClient *gptscript.GPTScript, serverConfig ServerConfig) ([]Webhook, error) {
	var result []Webhook
	webhookSeen := make(map[string]struct{})

	objs, err := wh.indexer.ByIndex("server-names", serverConfig.MCPServerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from MCP server index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-entry-names", serverConfig.MCPCatalogEntryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog entry index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("selectors", "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from selector index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-names", serverConfig.MCPCatalogName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	return result, nil
}

func (wh *WebhookHelper) appendWebhooks(ctx context.Context, gptClient *gptscript.GPTScript, namespace string, objs []any, seen map[string]struct{}, result []Webhook) []Webhook {
	var credEnv map[string]string
	result = slices.Grow(result, len(objs))

	for _, mwv := range objs {
		res, ok := mwv.(*v1.MCPWebhookValidation)
		if ok && res.Namespace == namespace && !res.Spec.Manifest.Disabled {
			manifest := res.Spec.Manifest

			// Use different key for deduplication based on hook type
			var seenKey string
			if manifest.SystemMCPServerName != "" {
				seenKey = fmt.Sprintf("system:%s/%s", manifest.SystemMCPServerName, manifest.ToolName)
			} else {
				seenKey = manifest.URL
			}

			if _, seen := seen[seenKey]; seen {
				continue
			}
			seen[seenKey] = struct{}{}

			displayName := manifest.Name
			if displayName == "" {
				displayName = res.Name
			}

			webhook := Webhook{
				Name:        res.Name,
				DisplayName: displayName,
				Definitions: manifest.Selectors.Strings(),
			}

			if manifest.SystemMCPServerName != "" {
				// SystemMCPServer hook - no URL, secret, or image needed
				webhook.SystemMCPServerName = manifest.SystemMCPServerName
				webhook.ToolName = manifest.ToolName
			} else {
				// Traditional URL webhook
				if credEnv == nil {
					cred, err := gptClient.RevealCredential(ctx, []string{system.MCPWebhookValidationCredentialContext}, res.Name)
					if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
						continue
					}
					credEnv = cred.Env
					if credEnv == nil {
						credEnv = make(map[string]string)
					}
				}
				webhook.URL = manifest.URL
				webhook.Secret = credEnv["secret"]
				webhook.Image = wh.defaultBaseImage
			}

			result = append(result, webhook)
		}
	}

	return result
}
