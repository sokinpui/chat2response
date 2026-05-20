package server

import (
	"os"

	"github.com/sokinpui/chat2response/pkg/proxy"
	"github.com/sokinpui/chat2response/pkg/types"
	"github.com/spf13/cobra"
)

type CliArgs struct {
	UpstreamFormat string
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

func Execute() {
	args := CliArgs{}
	rootCmd := &cobra.Command{
		Use:     "chat2response",
		Short:   "Adapter for old standard Chat Completions format API and new Responses format API",
		Example: `  chat2response --base-url https://api.anthropic.com/v1/messages`,
		Run: func(cmd *cobra.Command, _ []string) {
			var opts proxy.ProxyOptions
			host := "127.0.0.1"
			port := 9002
			cors := !args.NoCors

			if args.BaseURL != "" {
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
	flags.StringVar(&args.Host, "host", "127.0.0.1", "Bind host")
	flags.IntVarP(&args.Port, "port", "p", 9002, "Bind port (0 = random)")
	flags.StringVar(&args.APIVersion, "api-version", "", "Override anthropic-version header")
	flags.StringVar(&args.APIKey, "apikey", "", "Override upstream Authorization header")
	flags.StringVar(&args.Model, "model", "", "Override the model field in incoming requests")
	flags.BoolVar(&args.DropImages, "drop-images", false, "Drop images from user messages")
	flags.BoolVar(&args.NoCors, "no-cors", false, "Disable CORS headers")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
