package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sokinpui/chat2response/pkg/proxy"
	"github.com/sokinpui/chat2response/pkg/types"
	"github.com/sokinpui/chat2response/pkg/utils"
)

type CliArgs struct {
	UpstreamFormat string
	Config         string
	Host           string
	Port           int
	BaseURL        string
	APIVersion     string
	APIKey         string
	Model          string
	Cors           bool
	NoCors         bool
	DropImages     bool
	Help           bool
}

func ParseArgs(argv []string) CliArgs {
	out := CliArgs{Cors: true}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		take := func() string {
			if i+1 < len(argv) {
				i++
				return argv[i]
			}
			return ""
		}

		switch {
		case arg == "-h" || arg == "--help":
			out.Help = true
		case arg == "-p" || arg == "--port":
			out.Port, _ = strconv.Atoi(take())
		case arg == "--host":
			out.Host = take()
		case arg == "--upstream-format":
			out.UpstreamFormat = take()
		case arg == "--base-url":
			out.BaseURL = take()
		case arg == "--config":
			out.Config = take()
		case arg == "--api-version":
			out.APIVersion = take()
		case arg == "--apikey":
			out.APIKey = take()
		case arg == "--model":
			out.Model = take()
		case arg == "--drop-images":
			out.DropImages = true
		case arg == "--no-cors":
			out.NoCors = true
			out.Cors = false
		case strings.HasPrefix(arg, "--upstream-format="):
			out.UpstreamFormat = arg[len("--upstream-format="):]
		case strings.HasPrefix(arg, "--port="):
			out.Port, _ = strconv.Atoi(arg[len("--port="):])
		case strings.HasPrefix(arg, "--host="):
			out.Host = arg[len("--host="):]
		case strings.HasPrefix(arg, "--base-url="):
			out.BaseURL = arg[len("--base-url="):]
		case strings.HasPrefix(arg, "--api-version="):
			out.APIVersion = arg[len("--api-version="):]
		case strings.HasPrefix(arg, "--apikey="):
			out.APIKey = arg[len("--apikey="):]
		case strings.HasPrefix(arg, "--model="):
			out.Model = arg[len("--model="):]
		case strings.HasPrefix(arg, "--config="):
			out.Config = arg[len("--config="):]
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Printf("Unknown argument: %s\n", arg)
				out.Help = true
			}
		}
	}
	return out
}

func PrintHelp() {
	fmt.Print(`codeproxy - local Responses API proxy (Go version)

Usage:
  codeproxy --base-url <url> [options]
  codeproxy --config <file> [options]

Options:
  --base-url <url>         Upstream endpoint URL (required when not using --config)
  --upstream-format <fmt>  Upstream API format: anthropic | openai-chat
                           (optional; inferred from --base-url when omitted)
  --config <file>          Use a config file instead of CLI flags
  --host <host>            Bind host (default: 127.0.0.1)
  -p, --port <port>        Bind port (default: 8787; 0 = random)
  --api-version <ver>      Override anthropic-version header (anthropic only)
  --apikey <key>           Override upstream Authorization: Bearer <key>
  --model <name>           Override the model field in incoming requests
  --drop-images            Drop images from user messages
  --no-cors                Disable CORS headers
  -h, --help               Show help

Examples:
  codeproxy --base-url https://api.anthropic.com/v1/messages
  codeproxy --config config.json
`)
}

func LoadConfigAndApplyOverrides(configPath string, overrides CliArgs) (proxy.ProxyOptions, string, int, bool) {
	config, err := utils.LoadConfigFile(configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := utils.ValidateConfig(config); err != nil {
		fmt.Printf("Invalid config file: %v\n", err)
		os.Exit(1)
	}

	upstream := utils.GetCurrentUpstreamConfig(config)
	fmt.Printf("Loaded config from: %s\n", configPath)
	fmt.Printf("Current upstream: %s\n", config.CurrentUpstream)

	host := "127.0.0.1"
	if upstream.Host != nil {
		host = *upstream.Host
	}
	if overrides.Host != "" {
		host = overrides.Host
	}

	port := 8787
	if upstream.Port != nil {
		switch v := upstream.Port.(type) {
		case float64:
			port = int(v)
		case string:
			port, _ = strconv.Atoi(v)
		}
	}
	if overrides.Port != 0 {
		port = overrides.Port
	}

	cors := true
	if overrides.NoCors {
		cors = false
	}

	opts := proxy.ProxyOptions{
		UpstreamFormat:  upstream.Format,
		BaseURL:         upstream.BaseURL,
		APIVersion:      utils.Deref(upstream.APIVersion),
		Model:           utils.Deref(upstream.Model),
		DropImages:      utils.DerefBool(upstream.DropImages),
		ReasoningEffort: utils.Deref(upstream.ReasoningEffort),
		Thinking:        upstream.Thinking,
		TimeoutMs:       utils.DerefInt(upstream.TimeoutMs),
	}

	if overrides.BaseURL != "" {
		opts.BaseURL = overrides.BaseURL
	}
	if overrides.UpstreamFormat != "" {
		opts.UpstreamFormat = types.UpstreamFormat(overrides.UpstreamFormat)
	}
	if overrides.APIVersion != "" {
		opts.APIVersion = overrides.APIVersion
	}
	if overrides.Model != "" {
		opts.Model = overrides.Model
	}

	headers := make(map[string]string)
	for k, v := range config.Headers {
		headers[strings.ToLower(k)] = v
	}
	for k, v := range upstream.Headers {
		headers[strings.ToLower(k)] = v
	}
	if upstream.APIKey != nil {
		headers["authorization"] = "Bearer " + *upstream.APIKey
	}
	if overrides.APIKey != "" {
		headers["authorization"] = "Bearer " + overrides.APIKey
	}
	opts.DefaultHeaders = headers

	if upstream.Fallback != nil {
		if fb, ok := config.Upstreams[*upstream.Fallback]; ok {
			fbOpts := proxy.ProxyOptions{
				BaseURL:         fb.BaseURL,
				UpstreamFormat:  fb.Format,
				Model:           utils.Deref(fb.Model),
				APIVersion:      utils.Deref(fb.APIVersion),
				ReasoningEffort: utils.Deref(fb.ReasoningEffort),
				Thinking:        fb.Thinking,
			}
			fbHeaders := make(map[string]string)
			for k, v := range config.Headers {
				fbHeaders[strings.ToLower(k)] = v
			}
			for k, v := range fb.Headers {
				fbHeaders[strings.ToLower(k)] = v
			}
			if fb.APIKey != nil {
				fbHeaders["authorization"] = "Bearer " + *fb.APIKey
			}
			fbOpts.DefaultHeaders = fbHeaders
			opts.Fallback = &fbOpts
		}
	}

	return opts, host, port, cors
}

func Main(args []string) {
	cliArgs := ParseArgs(args[1:])

	if cliArgs.Help {
		PrintHelp()
		return
	}

	var opts proxy.ProxyOptions
	host := "127.0.0.1"
	port := 8787
	cors := true

	if cliArgs.Config != "" {
		opts, host, port, cors = LoadConfigAndApplyOverrides(cliArgs.Config, cliArgs)
	} else if cliArgs.BaseURL != "" {
		opts = proxy.ProxyOptions{
			UpstreamFormat: types.UpstreamFormat(cliArgs.UpstreamFormat),
			BaseURL:        cliArgs.BaseURL,
			APIVersion:     cliArgs.APIVersion,
			Model:          cliArgs.Model,
			DropImages:     cliArgs.DropImages,
		}
		if cliArgs.APIKey != "" {
			opts.DefaultHeaders = map[string]string{
				"authorization": "Bearer " + cliArgs.APIKey,
			}
		}
		if cliArgs.Host != "" {
			host = cliArgs.Host
		}
		if cliArgs.Port != 0 {
			port = cliArgs.Port
		}
		if cliArgs.NoCors {
			cors = false
		}
	} else {
		fmt.Println("Error: Either --config <file> or --base-url <url> is required")
		fmt.Println("")
		PrintHelp()
		os.Exit(1)
	}

	srv := NewServer(opts, host, port, cors)
	srv.Start()
}
