package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"

	"github.com/lmullen/legal-modernism/go/db"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
)

func init() {
	initLogger()
}

// prettyBytes formats a byte count as a human-readable string using binary prefixes.
func prettyBytes(b uint64) string {
	const (
		GiB = 1 << 30
		MiB = 1 << 20
		KiB = 1 << 10
	)
	switch {
	case b >= GiB:
		return fmt.Sprintf("%.1f GiB", float64(b)/GiB)
	case b >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(b)/MiB)
	case b >= KiB:
		return fmt.Sprintf("%.1f KiB", float64(b)/KiB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func main() {
	slog.Debug("starting diagnostics")

	// Report OS, architecture, and Go version
	slog.Info("runtime", "os", runtime.GOOS, "arch", runtime.GOARCH, "go_version", runtime.Version())

	// Report the hostname
	host, err := os.Hostname()
	if err != nil {
		slog.Error("error getting hostname", "error", err)
		os.Exit(1)
	}
	slog.Info("hostname", "host", host)

	// Report non-loopback IPv4 addresses across all network interfaces
	ifaces, err := psnet.Interfaces()
	if err != nil {
		slog.Error("failed to get network interfaces", "error", err)
	} else {
		var addrs []string
		for _, iface := range ifaces {
			for _, addr := range iface.Addrs {
				ip, _, err := net.ParseCIDR(addr.Addr)
				// Skip parse errors, loopback (127.x.x.x), and IPv6 addresses
				if err != nil || ip.IsLoopback() || ip.To4() == nil {
					continue
				}
				addrs = append(addrs, addr.Addr)
			}
		}
		slog.Info("IP addresses", "addresses", addrs)
	}

	// Report CPU cores and GOMAXPROCS
	slog.Info("available CPUs", "cpu_cores", runtime.NumCPU(), "gomaxprocs", runtime.GOMAXPROCS(0))

	vmem, err := mem.VirtualMemory()
	if err != nil {
		slog.Error("failed to get memory info", "error", err)
	} else {
		slog.Info("system memory", "total", prettyBytes(vmem.Total), "available", prettyBytes(vmem.Available))
	}

	// Check environment variables
	_, debugSet := os.LookupEnv("LAW_DEBUG")
	if _, ok := os.LookupEnv("LAW_DBSTR"); !ok {
		slog.Error("required environment variable not set", "variable", "LAW_DBSTR")
	} else {
		slog.Info("environment variables set", "LAW_DBSTR", true, "LAW_DEBUG", debugSet)
	}

	// Test database connectivity
	pool, err := db.Connect(context.Background())
	if err != nil {
		slog.Error("error connecting to database", "database", db.Host(), "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("successfully connected to database", "database", db.Host())

	slog.Debug("diagnostics complete")
}
