// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// azure_openai is the OpenAI-compatible adapter in its Azure mode: the
// deployment name goes in the path, the api-version in the query, and the key
// in the api-key header. The binding's location is the Azure region that
// processes the call, which core must recognise (azureregions.go).

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// azureAPIVersion is the generally available data-plane version that takes
// max_completion_tokens, json_schema output and tool calls.
const azureAPIVersion = "2024-10-21"

// azureDeploymentShape is an Azure deployment name. It becomes a path
// segment, so a separator or a query character may not reach it.
var azureDeploymentShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// azureMode is what makes an openAICompatClient speak to Azure.
type azureMode struct{ apiVersion string }

// validateAzurePlacement holds the fields an azure_openai binding is addressed
// by: the resource endpoint, the region and the deployment. profile is empty
// where the caller has none, and then only the region's recognition is held.
func validateAzurePlacement(label string, profile Profile, binding ProviderConfig) error {
	if binding.Provider != providerAzureOpenAI {
		return nil
	}
	if strings.TrimSpace(binding.BaseURL) == "" {
		return missingHostError{provider: providerAzureOpenAI, reason: noHostReason(label, providerAzureOpenAI)}
	}
	if err := azureEndpointError(label, binding.BaseURL); err != nil {
		return err
	}
	if err := azureRegionError(label, binding.Location); err != nil {
		return err
	}
	if profile == ProfileEUHosted && !azureRegionInEU(binding.Location) {
		country, _ := azureRegionCountry(binding.Location)
		return fmt.Errorf("ai: routing config: %s: profile eu_hosted needs an Azure region inside the EU, and %q is in %s; pick an EU region such as swedencentral or germanywestcentral",
			label, binding.Location, country)
	}
	if binding.Model != "" && !azureDeploymentShape.MatchString(binding.Model) {
		return fmt.Errorf("ai: routing config: %s: azure_openai model %q is not a deployment name: use the name you gave the deployment in Azure", label, binding.Model)
	}
	return nil
}

// azureRegionError refuses a region core cannot place in a country. It runs on
// every profile: where the data is processed is not something to guess.
func azureRegionError(label, region string) error {
	if region == "" {
		return fmt.Errorf("ai: routing config: %s: azure_openai needs a `location`: the Azure region of the resource, such as swedencentral", label)
	}
	if _, known := azureRegionCountry(region); !known {
		return fmt.Errorf("ai: routing config: %s: Azure region %q is not one this build knows, so it is blocked; pick a listed region, because core cannot say which country it processes in", label, region)
	}
	return nil
}

// azureEndpointError holds the host to one Azure OpenAI resource: https, a
// name under openai.azure.com, no path, port or credential. The API key goes to
// this host, so a host Azure does not own never receives it.
func azureEndpointError(label, baseURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	host := ""
	if err == nil {
		host = strings.ToLower(parsed.Hostname())
	}
	resource := strings.TrimSuffix(host, azureOpenAIHostSuffix)
	valid := err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.User == nil && parsed.Port() == "" &&
		strings.Trim(parsed.Path, "/") == "" && parsed.RawQuery == "" && parsed.Fragment == "" &&
		strings.HasSuffix(host, azureOpenAIHostSuffix) && resource != "" && !strings.Contains(resource, ".")
	if !valid {
		return fmt.Errorf("ai: routing config: %s: azure_openai base_url must be the resource endpoint, https://<resource>.openai.azure.com, with no path", label)
	}
	return nil
}

// noHostReason is the refusal for a chat lane bound to an adapter that has no
// default host, worded for what that adapter's host is.
func noHostReason(label, provider string) string {
	if provider == providerAzureOpenAI {
		return fmt.Sprintf("ai: routing config: %s binds azure_openai with no base_url: give it the resource endpoint, e.g. https://my-resource.openai.azure.com", label)
	}
	return fmt.Sprintf("ai: routing config: %s binds openai_compatible with no base_url: "+
		"give it the vendor host root, with no version segment (the adapter adds /v1), "+
		"e.g. https://openrouter.ai/api", label)
}

// validateAzureEntry holds an azure_openai provider entry to its shape. A
// lane that binds it adds the profile rule on top.
func validateAzureEntry(settings ProviderSettings) error {
	if settings.BaseURL != "" {
		if err := azureEndpointError("providers: "+providerAzureOpenAI, settings.BaseURL); err != nil {
			return err
		}
	}
	if settings.Location != "" {
		return azureRegionError("providers: "+providerAzureOpenAI, settings.Location)
	}
	return nil
}

// selectAzureOpenAI builds the azure_openai client. Its placement is checked
// again here, so a binding that bypassed the write-time rule still cannot send
// data to a region core does not recognise.
//
//nolint:ireturn // SelectBrain's own return shape, so a refusal stays a nil interface
func selectAzureOpenAI(cfg ProviderConfig, keys config.Lookup, httpc *http.Client) (model.Client, error) {
	key := cloudKey(providerAzureOpenAI, keys)
	if key == "" {
		return nil, byokKeyRequired(providerAzureOpenAI)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("%w: azure_openai (the resource endpoint, e.g. https://my-resource.openai.azure.com)", errNoBaseURL)
	}
	if err := validateAzurePlacement("the binding", "", cfg); err != nil {
		return nil, err
	}
	return &openAICompatClient{
		http:            netguard.NoRedirect(httpc),
		baseURL:         strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:          key,
		defaultModel:    cfg.Model,
		attachmentMIMEs: carriageFor(cfg.Input),
		azure:           &azureMode{apiVersion: azureAPIVersion},
	}, nil
}

// requestURL is where one call goes. Azure names the deployment in the path and
// wants the api-version on every call; everything else appends the OpenAI path.
func (c *openAICompatClient) requestURL(deployment, path string) (string, error) {
	if c.azure == nil {
		return c.baseURL + path, nil
	}
	if !azureDeploymentShape.MatchString(deployment) {
		return "", errors.New("ai: azure_openai: no deployment name: bind a model that names the Azure deployment")
	}
	return c.baseURL + "/openai/deployments/" + url.PathEscape(deployment) + strings.TrimPrefix(path, "/v1") +
		"?api-version=" + url.QueryEscape(c.azure.apiVersion), nil
}

// authorize sets the credential header the host reads: api-key on Azure, a
// bearer token elsewhere. An empty key sends nothing (a local vLLM).
func (c *openAICompatClient) authorize(r *http.Request) error {
	if c.azure == nil {
		return bearerAuthorizer(c.apiKey)(r)
	}
	r.Header.Set("api-key", c.apiKey)
	return nil
}

// modelsURL is the catalog address: Azure lists the models a resource can
// deploy under /openai/models, the rest under /v1/models.
func (c *openAICompatClient) modelsURL() string {
	if c.azure == nil {
		return c.baseURL + "/v1/models"
	}
	return c.baseURL + "/openai/models?api-version=" + url.QueryEscape(c.azure.apiVersion)
}
