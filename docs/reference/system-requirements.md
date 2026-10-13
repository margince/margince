<!-- prose:plain -->
# System requirements

An installation runs in one of two shapes: **single node** or **separate
nodes**. On a single node, all
services run on one host. With separate nodes, the API, the worker, the web
server and the database each run on their own node. The two shapes use the
same installation steps and the same config.

The sizes below apply when the AI features use a cloud provider. A model on
the installation's own hardware needs more memory and can need a GPU (see
*AI models on your own hardware*). The AI features are optional: with no model
bound, the rest of the product works as normal.

## Software

| What you need | Notes |
|---|---|
| **Postgres 16** with **pgvector** ≥ 0.5.0 | Postgres does not trust the `vector` extension. A superuser must install it one time (`scripts/deploy/db-bootstrap.sql`). |
| **Redis 7.0 to 7.2** | It needs streams with consumer groups. **Valkey** works too, but we do not test it. |
| **A proxy in front that ends TLS** | Serve the API and the web under one host name. |
| **A correct system clock** | Retention, automation triggers and job schedules use the system time. |
| **An object store** that works with the `S3` API | Files attached to records, company images, offer PDF files, file custom fields and CSV import keep their data in it. |
| Outbound HTTPS | Optional, for some features. The core CRM, sign-in and the license check work in full with no network. |
| A model endpoint | Optional, only for the AI features: a cloud provider with your own key, or your own Ollama / vLLM host. See below. |

The API, worker and web containers keep no lasting local state. Postgres,
Redis and the object store keep all lasting state.

## Data size levels

| Level | Contacts | Companies | Activities |
|---|---|---|---|
| **Small** | 10,000 | 1,000 | 20,000 |
| **Mid-market** | 250,000 | 10,000 | 500,000 |

Choose the level by the number of contacts. Then decide if AI search and
retrieval is on. The embedding store is about 20 times the size of
the CRM data. So this choice changes the numbers more than the level does.
The 20 times applies to the default embedding size of 1536 values. The
store size grows in step with the `dimensions` you set.

## Single node

All services run on one host. Containers are optional. The sizes do not
change.

| Level | Retrieval | vCPU | RAM | Disk |
|---|---|---|---|---|
| Small | off | 2 | 4 GB | 20 GB |
| Small | on | 2 | 8 GB | 40 GB |
| Mid-market | off | 4 | 8 GB | 50 GB |
| Mid-market | on | 8 | 32 GB | 200 GB |

- **Set `shared_buffers` by hand.** The Postgres defaults expect a machine
  that runs only Postgres. Here, Postgres shares the memory with the other
  processes. If the value stays at the size for a host with only Postgres, the
  node runs out of memory and moves memory pages to disk. You see slow page
  loads, not an error message.
- **A restart stops the service for a time.** The API applies the migrations
  at start. So this shape has no upgrade one node at a time. All services
  fail together.

## Separate nodes

| Node | vCPU | RAM |
|---|---|---|
| **Postgres** | 2 (small) to 8 (mid-market) | 4 GB (small) to 32 GB (mid-market, retrieval on) |
| **API** | 2 | 2 GB for each replica |
| **worker** | 2 | 4 GB for each replica |
| **web** | 1 | 512 MB |
| **Redis / Valkey** | 1 | 2 GB |

For the database disk, use the single node table above. The other nodes do
not need disk space.

- **Keep the API in the database's zone.** Each page
  load waits for the time a call takes there and back. Putting them in
  different places makes the product slow. More CPU does not fix this.
- Make sure that the API and the worker can connect to Redis and to the
  object store.

## Database settings

- **`max_connections`**: allow **40** connections for one API and one
  worker. Add **19** for each extra API replica. Add **16** for each
  extra worker replica. Add room for admin and health check
  sessions.
- **Run connection pools in transaction mode.** The tenant limit
  binds to one transaction. Statement pooling breaks that limit. Session
  pooling wastes the pool.
- **Plan four times the normal data size.** This covers
  `write-ahead` logs, space that old rows still hold, and building indexes again. Add space for backups.
- **The retention posture limits how much data grows.** The default policy makes old
  records with no names and erases them on a yearly schedule. An installation with
  `retain_only` never deletes data. Size such an installation on how long it
  keeps data.

## Network

The API and the web are behind one proxy with **one host name**. The
web app, the MCP client handshake and the OAuth consent flow need
this.

Only some features need outbound access. These are the AI provider, the FX
and model price sources, the Gmail / Microsoft capture connectors, outbound
webhook endpoints, and the SMTP relay. An installation with no outside
network is possible. Without an AI key, the AI lanes stop, and the other
features go on.

## AI providers

The product runs no model of its own. A cloud provider runs on your own
key; Ollama and vLLM run on your own host and need none. Supported:
**Anthropic** (Claude), **OpenAI** (GPT), **Google** (Gemini), and any
vendor with the **OpenAI API shape** (Mistral, DeepSeek, Groq, OpenRouter, a
gateway, …).

- The API and the worker both call the model. Give both of them outbound
  access to every endpoint you bind. The API of a cloud vendor is HTTPS only.
  An Ollama, vLLM or OpenAI API shape endpoint on your own network can be plain
  HTTP. Use HTTPS for any other endpoint.
- The embeddings feature is bound on its own, not with chat and can name its own
  endpoint. The API calls it for search by meaning, the worker to build the
  index and build it again, so both need that endpoint too. A vendor with chat
  only cannot serve it. A local embedding model is another way.

## AI models on your own hardware

**Ollama** and **vLLM** serve a model on your own hardware. No key is
needed, and these are the only choices in the `sovereign` profile (zero
egress).

The model host comes on top of the node tables above. The sizes below take
the default model class: Gemma 3 (`gemma3` on Ollama, `google/gemma-3-12b-it`
on vLLM). They also take the 40,960-token context window the Ollama adapter
asks for, and an embedding model (for example `bge-m3`) on the same host.
Plan **50 GB disk** for model files.

| Host | Hardware | Serves |
|---|---|---|
| **Smallest** | GPU with 8 GB memory, 16 GB RAM | A quantized `4B` model plus the embedding model. Good enough to try the AI features; expect bad results when it reads fields out of a document. |
| **Best choice** | GPU with 24 GB memory (`RTX 4090`, `L4`, `A10`), 32 GB RAM | A quantized `12B`–`27B` model with the full context window, the embedding model next to it, and fast answers while a user waits. |
| **Many calls at once, or not quantized** | 48 GB GPU memory or more (`L40S`, `RTX 6000 Ada`, `2 × 24 GB`), 64 GB RAM | A `12B` model that is not quantized on vLLM, or more calls at once for the same work. |

- A host with only a CPU works, but only for background tasks. Answers take
  minutes, and features a user waits on become too slow to use.
- The model server must support output that follows a JSON schema. Most tasks
  hold the answer to a schema.
- A task can move up from a local model to a cloud tier. The embeddings
  lane is bound on its own, not with the tiers. For an installation that is local in
  full, bind every tier *and* the embeddings lane to a local model. Or use the
  `sovereign` profile, which refuses a cloud provider on every lane at start.

## Running many copies

The API and the worker scale to many replicas with no config.
API copies that run at once share the event backlog. They do not each do it
again. Two starts at the same time cannot both run the schema migration. Scheduled
jobs run one time in the cluster, for any number of workers.

## Browsers

Use a current version of Chrome, Edge, Firefox or Safari.
