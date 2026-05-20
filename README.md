# chat2response

Adapter for OpenAI Chat Completions format API and Anthropic format API to OpenAI Responses format API.

## Installation

### From source

Ensure you have [Go](https://go.dev/) installed.

```bash
go install github.com/sokinpui/chat2response/cmd/chat2response@latest
```

### Local Build

```bash
git clone https://github.com/sokinpui/chat2response.git
cd chat2response
go build -o chat2response ./cmd/chat2response
```

## Usage

Start the proxy server by specifying the upstream base URL and optionally an API key.

### Basic Usage (Anthropic)

```bash
./chat2response --base-url https://api.anthropic.com/v1/messages --apikey your-api-key
```

### Basic Usage (OpenAI)

```bash
./chat2response --base-url https://api.openai.com/v1/chat/completions --apikey your-api-key
```

By default, the server runs on `127.0.0.1:9002`.

### CLI Options

| Flag                | Description                                       | Default       |
| ------------------- | ------------------------------------------------- | ------------- |
| `--base-url`        | Upstream endpoint URL (Required)                  |               |
| `--upstream-format` | Upstream API format: `anthropic` or `openai-chat` | Auto-detected |
| `--host`            | Bind host                                         | `127.0.0.1`   |
| `--port`, `-p`      | Bind port                                         | `9002`        |
| `--apikey`          | Override upstream Authorization header            |               |
| `--api-version`     | Override `anthropic-version` header               | `2023-06-01`  |
| `--model`           | Override the model field in incoming requests     |               |
| `--drop-images`     | Drop images from user messages                    | `false`       |
| `--no-cors`         | Disable CORS headers                              | `false`       |

## Running

Once the server is running, your Request is translate and proxy to your OpenAI Chat Completions or Anthropic endpoint, and the response is translated back to OpenAI Responses format.

Example with `curl`:

```bash
curl http://127.0.0.1:9002/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20240620",
    "input": "Why the sky is blue?"
  }'
```

### Headers Passthrough

The proxy passes through most headers to the upstream, except for hop-by-hop and certain client-specific headers (e.g., `Host`, `Content-Length`, `OpenAI-*`).
