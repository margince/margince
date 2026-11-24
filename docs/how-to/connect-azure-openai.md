<!-- prose:plain -->
# Connect Azure OpenAI (BYOK)

`azure_openai` is the `openai_compatible` adapter in its Azure form. It puts the deployment name in the
URL path, adds `api-version` to the query, and sends the key in the `api-key` header. The sign-in is the
static key of the resource, and nothing else.

1. In Azure, create the model deployment and note its name and the resource endpoint.
2. In Settings → AI → AI models → Providers, paste the key into the `azure_openai` row. Open its sheet and
   set the **Host** (`https://<resource>.openai.azure.com`) and the **Azure region** of the resource.
3. Bind a tier to `azure_openai` and write the deployment name as the model.

```yaml
profile: eu_hosted
providers:
  azure_openai: { base_url: https://contoso.openai.azure.com, location: swedencentral }
tiers:
  premium: { provider: azure_openai, model: my-gpt-deployment }
embeddings: { provider: azure_openai, model: my-embedding-deployment }
```

Margince sends the key only to a host that ends in `.openai.azure.com`. It also knows which country each
Azure region is in, from the public Azure list of regions. A region it does not know is blocked
on every profile, so data cannot leave through a region nobody mapped. Under `eu_hosted` the region must
also be inside the EU: `swedencentral`, `germanywestcentral` and `westeurope` pass, while `uksouth`,
`norwayeast`, `switzerlandnorth` and `eastus` do not. `sovereign` refuses `azure_openai` like every
cloud provider. A region Azure adds after this build is blocked until a Margince release lists it.

Use a Standard deployment, which runs in the region you named. A deployment of type `GlobalStandard` or `DataZoneStandard` runs
outside it, and Margince cannot see which type a deployment has.

A 429 answer from Azure uses the same handling as every other provider. A rate limit and an empty quota are
told apart there.

## When it does not work

| You see | Do this |
|---|---|
| 422 `Azure region … is not one this build knows, so it is blocked` | Margince cannot say which country the region is in. Use a region from the public Azure list of regions that this build knows. |
| 422 `… profile eu_hosted needs an Azure region inside the EU` | The region is outside the EU. Choose an EU region such as `swedencentral`, or declare `cloud_frontier`. |

Other errors, and the key and host rules every provider shares, are in [connect-a-cloud-model-provider.md](connect-a-cloud-model-provider.md).
