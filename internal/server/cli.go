package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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
}

func loadConfigAndApplyOverrides(configPath string, overrides CliArgs) (proxy.ProxyOptions, string, int, bool) {
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
			fmt.Sscanf(v, "%d", &port)
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

func Execute() {
	args := CliArgs{}
	rootCmd := &cobra.Command{
		Use:   "chat2response",
		Short: "Adapter for old standard Chat Completions format API and new Responses format API",
		Example: `  chat2response --base-url https://api.anthropic.com/v1/messages
  chat2response --config config.json`,
		Run: func(cmd *cobra.Command, _ []string) {
			var opts proxy.ProxyOptions
			host := "127.0.0.1"
			port := 8787
			cors := !args.NoCors

			if args.Config != "" {
				opts, host, port, cors = loadConfigAndApplyOverrides(args.Config, args)
			} else if args.BaseURL != "" {
				opts = proxy.ProxyOptions{
					UpstreamFormat: types.UpstreamFormat(args.UpstreamFormat),
					BaseURL:        args.BaseURL,
					APIVersion:     args.APIVersion,
					Model:          args.Model,
					DropImages:     args.DropImages,
				}
				if args.APIKey != "" {
					opts.DefaultHeaders = map[string]string{
						"authorization": "Bearer " + args.APIKey,
					}
				}
				if args.Host != "" {
					host = args.Host
				}
				if args.Port != 0 {
					port = args.Port
				}
			} else {
				cmd.Help()
				os.Exit(1)
			}

			srv := NewServer(opts, host, port, cors)
			srv.Start()
		},
	}

	flags := rootCmd.Flags()
	flags.StringVar(&args.BaseURL, "base-url", "", "Upstream endpoint URL")
	flags.StringVar(&args.UpstreamFormat, "upstream-format", "", "Upstream API format: anthropic | openai-chat")
	flags.StringVar(&args.Config, "config", "", "Use a config file instead of CLI flags")
	flags.StringVar(&args.Host, "host", "127.0.0.1", "Bind host")
	flags.IntVarP(&args.Port, "port", "p", 8787, "Bind port (0 = random)")
	flags.StringVar(&args.APIVersion, "api-version", "", "Override anthropic-version header")
	flags.StringVar(&args.APIKey, "apikey", "", "Override upstream Authorization header")
	flags.StringVar(&args.Model, "model", "", "Override the model field in incoming requests")
	flags.BoolVar(&args.DropImages, "drop-images", false, "Drop images from user messages")
	flags.BoolVar(&args.NoCors, "no-cors", false, "Disable CORS headers")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
