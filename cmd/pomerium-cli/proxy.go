package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/elazarl/goproxy"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/pomerium/pomerium/internal/log"
)

var proxyCmdOptions struct {
	listen      string
	pomeriumURL string
}

func init() {
	addTLSFlags(proxyCmd)
	flags := proxyCmd.Flags()
	flags.StringVar(&proxyCmdOptions.listen, "listen", "127.0.0.1:3128",
		"local address to start a listener on")
	flags.StringVar(&proxyCmdOptions.pomeriumURL, "pomerium-url", "",
		"the URL of the pomerium server to connect to")
	rootCmd.AddCommand(proxyCmd)
}

var proxyCmd = &cobra.Command{
	Use:  "proxy",
	Args: cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		l := zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = os.Stderr
		})).With().Timestamp().Logger()
		log.SetLogger(&l)

		proxy := goproxy.NewProxyHttpServer()
		proxy.Logger = &l
		proxy.Verbose = true

		// HTTPS
		proxy.OnRequest().HijackConnect(hijackProxyConnect)

		// HTTP
		proxy.OnRequest().DoFunc(handleProxyRequest)

		srv := &http.Server{
			Addr: proxyCmdOptions.listen,
			Handler: proxy,
		}

		// TODO: This is just copied and slightly adopted, might not make 100% sense
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-c
			cancel()
			srv.Shutdown(ctx)
		}()

		l.Info().Msgf("Proxy running at %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("proxy failed to start: %v", err)
		}
		return nil
	},
}

func hijackProxyConnect(req *http.Request, client net.Conn, ctx *goproxy.ProxyCtx) {
	dst := req.RequestURI
	log.Printf("Creating new tunnel to %q", dst)
	defer client.Close()
	tun, err := newTCPTunnel(dst, proxyCmdOptions.pomeriumURL)
	if err != nil {
		log.Printf("Failed to create TCP tunnel: %v", err)
	}

	// TODO: Correct contexth here
	client.Write([]byte("HTTP/1.1 200 Connection established\n\n"))
	if err := tun.Run(context.Background(), client); err != nil {
		log.Printf("Failed to run TCP tunnel: %v", err)
	}
}

func handleProxyRequest(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	// TODO: Handle normal HTTP request proxying here
	panic("HTTP requests not implemented")
	return r, nil
}
