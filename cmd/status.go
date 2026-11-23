package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
	"github.com/nerdneilsfield/aumbry/internal/client"
)

func newStatusCommand() *cobra.Command {
	var profileName string
	var asJSON bool
	command := &cobra.Command{
		Use:   "status",
		Short: "Show remote server version, health, resource usage, and content stats",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			profile, err := resolveProfile(profileName)
			if err != nil {
				return err
			}
			st, raw, err := client.GetStatus(command.Context(), profile.Server, profile.Token, nil)
			if err != nil {
				return err
			}
			if asJSON {
				_, err = command.OutOrStdout().Write(client.Indent(raw))
				return err
			}
			return writeFormattedStatus(command.OutOrStdout(), profile.Server, st, buildinfo.Current())
		},
	}
	flags := command.Flags()
	flags.StringVar(&profileName, "profile", "", "configuration profile (default: the active profile)")
	flags.BoolVar(&asJSON, "json", false, "output raw server status JSON")
	return command
}

func writeFormattedStatus(w io.Writer, serverURL string, st client.ServerStatus, local buildinfo.Info) error {
	sqliteCheck := st.Checks["sqlite"]
	if sqliteCheck == "" {
		sqliteCheck = "unknown"
	}
	storageCheck := st.Checks["storage"]
	if storageCheck == "" {
		storageCheck = "unknown"
	}

	memSummary := fmt.Sprintf("heap %s alloc / %s in-use / %s sys",
		formatBytes(st.Runtime.HeapAllocBytes),
		formatBytes(st.Runtime.HeapInuseBytes),
		formatBytes(st.Runtime.SysBytes),
	)
	if st.Runtime.ProcessRSSBytes > 0 {
		memSummary = fmt.Sprintf("rss %s (%s)", formatBytes(st.Runtime.ProcessRSSBytes), memSummary)
	}

	sqliteBytes := uint64(max(int64(0), st.Storage.SQLiteBytes))
	assetBytes := uint64(max(int64(0), st.Counts.AssetBytes))
	maxIngestBytes := uint64(max(int64(0), st.Config.MaxIngestBodyBytes))

	revLimit := "unlimited"
	if st.Config.MaxRevisionsPerItem > 0 {
		revLimit = fmt.Sprintf("%d", st.Config.MaxRevisionsPerItem)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Server:    %s\n", strings.TrimRight(strings.TrimSpace(serverURL), "/"))
	fmt.Fprintf(&b, "Health:    %s (sqlite=%s storage=%s)\n", st.Status, sqliteCheck, storageCheck)
	fmt.Fprintf(&b, "Uptime:    %s (started %s)\n", formatDurationSeconds(st.Runtime.UptimeSeconds), emptyFallback(st.Runtime.StartedAt, "unknown"))
	fmt.Fprintf(&b, "ServerVer: %s (commit %s, built %s)\n", displayVersionTag(st.Build.Version), emptyFallback(st.Build.Commit, "none"), emptyFallback(st.Build.BuildTime, "unknown"))
	fmt.Fprintf(&b, "CLIVer:    %s (commit %s, built %s)\n", displayVersionTag(local.Version), emptyFallback(local.Commit, "none"), emptyFallback(local.BuildTime, "unknown"))
	if normalizeVersionTag(local.Version) != normalizeVersionTag(st.Build.Version) {
		fmt.Fprintf(&b, "Note:      CLI (%s) and server (%s) versions differ\n", displayVersionTag(local.Version), displayVersionTag(st.Build.Version))
	}
	fmt.Fprintf(&b, "Runtime:   %s %s/%s, goroutines=%d, gc=%d\n", emptyFallback(st.Runtime.GoVersion, "unknown"), st.Runtime.GOOS, st.Runtime.GOARCH, st.Runtime.Goroutines, st.Runtime.GCCount)
	fmt.Fprintf(&b, "CPU:       %.1f%% across %d cores (%.2fs total)\n", st.Runtime.CPUPercent, st.Runtime.NumCPU, st.Runtime.CPUSecondsTotal)
	fmt.Fprintf(&b, "Memory:    %s\n", memSummary)
	fmt.Fprintf(&b, "Storage:   %s (sqlite %s)\n", emptyFallback(st.Storage.Backend, "unknown"), formatBytes(sqliteBytes))
	fmt.Fprintf(&b, "Content:   dirs=%d (%d protected), items=%d (%d published, %d drafts), revisions=%d, assets=%d (%s), dir_tokens=%d\n",
		st.Counts.Directories, st.Counts.ProtectedDirectories,
		st.Counts.Items, st.Counts.PublishedItems, st.Counts.DraftItems,
		st.Counts.Revisions,
		st.Counts.Assets, formatBytes(assetBytes),
		st.Counts.DirectoryTokens,
	)
	fmt.Fprintf(&b, "Activity:  ingest=%d/%d, mcp_sessions=%d (anon=%d, anon_posts=%d)\n",
		st.Activity.IngestInFlight, st.Activity.IngestLimit,
		st.Activity.MCPActiveSessions, st.Activity.MCPAnonSessions, st.Activity.MCPAnonPostsInFlight,
	)
	fmt.Fprintf(&b, "Limits:    max_ingest=%s, max_revisions=%s, remote_images=%t, public_url=%t, trusted_proxies=%d\n",
		formatBytes(maxIngestBytes), revLimit, st.Config.AllowRemoteImages, st.Config.PublicURLConfigured, st.Config.TrustedProxiesCount,
	)
	_, err := io.WriteString(w, b.String())
	return err
}

func normalizeVersionTag(v string) string {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return "dev"
	}
	return s
}

func displayVersionTag(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return "dev"
	}
	if s[0] >= '0' && s[0] <= '9' {
		return "v" + s
	}
	return s
}

func emptyFallback(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	val := float64(bytes) / float64(unit)
	idx := 0
	for val >= float64(unit) && idx < len(units)-1 {
		val /= float64(unit)
		idx++
	}
	if val >= 100 {
		return fmt.Sprintf("%.0f %s", val, units[idx])
	}
	if val >= 10 {
		return fmt.Sprintf("%.1f %s", val, units[idx])
	}
	return fmt.Sprintf("%.2f %s", val, units[idx])
}

func formatDurationSeconds(seconds int64) string {
	if seconds <= 0 {
		return "0s"
	}
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
