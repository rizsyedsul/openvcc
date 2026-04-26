package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/syedsumx/openvcc/internal/chaos"
	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/engine"
	"github.com/syedsumx/openvcc/internal/log"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "openvcc",
		Short:         "Open VCC: open-source virtual cloud connector",
		Long:          "Open VCC is an open-source virtual cloud connector for VM workloads.",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newEngineCmd(), newChaosCmd(), newVersionCmd())
	return root
}

func newChaosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chaos",
		Short: "Drive synthetic failover scenarios and emit signed reports",
	}
	cmd.AddCommand(newChaosRunCmd(), newChaosScheduleCmd())
	return cmd
}

func newChaosScheduleCmd() *cobra.Command {
	var (
		adminURL    string
		adminToken  string
		proxyURL    string
		failClouds  []string
		duration    time.Duration
		every       time.Duration
		rps         int
		concurrency int
		outDir      string
		prefix      string
		maxRuns     int
		stopOnFail  bool
	)
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Run chaos on an interval, archiving each report to disk",
		Long: `Loop chaos.Run on a fixed interval, rotating through fail-clouds and
writing each report into --output-dir as a timestamped JSON file.
Designed to be wrapped in cron / systemd timer / kubernetes CronJob;
exits non-zero on the first failed verdict if --stop-on-fail is set.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if adminToken == "" {
				adminToken = os.Getenv("OPENVCC_ADMIN_TOKEN")
			}
			if len(failClouds) == 0 {
				return fmt.Errorf("at least one --fail cloud is required")
			}
			sink := &chaos.FileSink{Dir: outDir, Prefix: prefix}
			t := time.NewTicker(every)
			defer t.Stop()
			runs := 0
			tick := func() error {
				cloud := failClouds[runs%len(failClouds)]
				report, err := chaos.Run(cmd.Context(), chaos.Options{
					AdminURL:    adminURL,
					AdminToken:  adminToken,
					ProxyURL:    proxyURL,
					FailCloud:   cloud,
					Duration:    duration,
					RPS:         rps,
					Concurrency: concurrency,
					Sink:        sink,
				})
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stdout, "[chaos] cloud=%s result=%s lag=%s\n",
					cloud, report.Result, report.FailoverLag)
				if stopOnFail && report.Result != "pass" {
					return fmt.Errorf("chaos failed for cloud=%s: %s", cloud, report.Reason)
				}
				return nil
			}
			if err := tick(); err != nil {
				return err
			}
			runs++
			for {
				if maxRuns > 0 && runs >= maxRuns {
					return nil
				}
				select {
				case <-cmd.Context().Done():
					return nil
				case <-t.C:
					if err := tick(); err != nil {
						return err
					}
					runs++
				}
			}
		},
	}
	cmd.Flags().StringVar(&adminURL, "admin-url", "http://localhost:8081", "Engine admin API URL")
	cmd.Flags().StringVar(&adminToken, "admin-token", "", "Bearer token (default: $OPENVCC_ADMIN_TOKEN)")
	cmd.Flags().StringVar(&proxyURL, "proxy-url", "http://localhost:8080", "Engine proxy URL")
	cmd.Flags().StringSliceVar(&failClouds, "fail", nil, "Cloud(s) to rotate failure across (e.g. aws,azure)")
	cmd.Flags().DurationVar(&duration, "duration", 30*time.Second, "Failure window per run")
	cmd.Flags().DurationVar(&every, "every", 24*time.Hour, "Interval between runs")
	cmd.Flags().IntVar(&rps, "rps", 50, "Synthetic requests per second")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Concurrent driver workers")
	cmd.Flags().StringVar(&outDir, "output-dir", "./chaos-reports", "Directory for archived reports")
	cmd.Flags().StringVar(&prefix, "prefix", "openvcc-chaos", "Filename prefix")
	cmd.Flags().IntVar(&maxRuns, "max-runs", 0, "Stop after N runs (0 = run forever)")
	cmd.Flags().BoolVar(&stopOnFail, "stop-on-fail", false, "Exit non-zero on the first non-passing run")
	_ = cmd.MarkFlagRequired("fail")
	return cmd
}

func newChaosRunCmd() *cobra.Command {
	var (
		adminURL    string
		adminToken  string
		proxyURL    string
		failCloud   string
		duration    time.Duration
		rps         int
		concurrency int
		outPath     string
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Fail one cloud, drive traffic, verify failover, write a JSON report",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if adminToken == "" {
				adminToken = os.Getenv("OPENVCC_ADMIN_TOKEN")
			}
			out := os.Stdout
			if outPath != "" && outPath != "-" {
				f, err := os.Create(outPath)
				if err != nil {
					return err
				}
				defer f.Close()
				out = f
			}
			report, err := chaos.Run(cmd.Context(), chaos.Options{
				AdminURL:    adminURL,
				AdminToken:  adminToken,
				ProxyURL:    proxyURL,
				FailCloud:   failCloud,
				Duration:    duration,
				RPS:         rps,
				Concurrency: concurrency,
				Output:      out,
			})
			if err != nil {
				return err
			}
			if report.Result != "pass" {
				return fmt.Errorf("chaos run did not pass: %s", report.Reason)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&adminURL, "admin-url", "http://localhost:8081", "Engine admin API URL")
	cmd.Flags().StringVar(&adminToken, "admin-token", "", "Bearer token (default: $OPENVCC_ADMIN_TOKEN)")
	cmd.Flags().StringVar(&proxyURL, "proxy-url", "http://localhost:8080", "Engine proxy URL to drive traffic at")
	cmd.Flags().StringVar(&failCloud, "fail", "", "Cloud label to fail (e.g. aws)")
	cmd.Flags().DurationVar(&duration, "duration", 30*time.Second, "How long to keep the cloud failed")
	cmd.Flags().IntVar(&rps, "rps", 50, "Synthetic requests per second")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Concurrent driver workers")
	cmd.Flags().StringVarP(&outPath, "output", "o", "-", "Report output path ('-' for stdout)")
	_ = cmd.MarkFlagRequired("fail")
	return cmd
}

func newEngineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "engine",
		Short: "Engine subcommands (serve, validate)",
	}
	cmd.AddCommand(newServeCmd(), newValidateCmd())
	return cmd
}

func newServeCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the Open VCC engine",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			logger, err := log.New(cfg.Log.Level, cfg.Log.Format, os.Stderr)
			if err != nil {
				return err
			}
			eng, err := engine.New(cfg, cfgPath, logger)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			logger.Info("openvcc engine starting",
				"version", version, "commit", commit, "date", date,
				"backends", len(cfg.Backends), "strategy", cfg.Strategy,
			)
			return eng.Run(ctx)
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "openvcc.yaml", "Path to openvcc.yaml")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func newValidateCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Parse and validate the engine config (exits non-zero on error)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "ok: %d backend(s), strategy=%s\n", len(cfg.Backends), cfg.Strategy)
			return nil
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "openvcc.yaml", "Path to openvcc.yaml")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(os.Stdout, "openvcc %s (commit=%s, date=%s, %s/%s)\n",
				version, commit, date, runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}

