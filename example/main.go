package main

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	port int
)

func main() {
	c := &cobra.Command{
		Use:   "apic",
		Short: "apic example apps",
	}

	c.PersistentFlags().IntVarP(&port, "port", "p", 4321, "server port")

	c.AddCommand(&cobra.Command{
		Use:   "ws-client",
		Short: "example websocket client",
		RunE:  runWsClient,
	})

	c.AddCommand(&cobra.Command{
		Use:   "ws-server",
		Short: "example websocket server, proxying some upstream",
		RunE:  runWsServer,
	})

	if err := c.Execute(); err != nil {
		os.Exit(1)
	}
}

func runWsClient(c *cobra.Command, args []string) error {
	return nil
}
