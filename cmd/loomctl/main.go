package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const version = "2.0.0"

var (
	serverURL    string
	outputFormat string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "loomctl",
		Short: "Loom CLI - interact with your Loom server",
		Long: `loomctl is a command-line interface for interacting with Loom servers.
All output is structured JSON by default (pipe through jq for human-readable formatting).`,
		Version: version,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&serverURL, "server", "s", getDefaultServer(), "Loom server URL")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "json", "Output format: json, table")

	// Add subcommands
	rootCmd.AddCommand(newBeadCommand())
	rootCmd.AddCommand(newContainerCommand())
	rootCmd.AddCommand(newWorkflowCommand())
	rootCmd.AddCommand(newAgentCommand())
	rootCmd.AddCommand(newProjectCommand())
	rootCmd.AddCommand(newLogCommand())
	rootCmd.AddCommand(newStatusCommand())
	rootCmd.AddCommand(newMetricsCommand())
	rootCmd.AddCommand(newConversationCommand())
	rootCmd.AddCommand(newAnalyticsCommand())
	rootCmd.AddCommand(newConfigCommand())
	rootCmd.AddCommand(newEventCommand())
	rootCmd.AddCommand(newExportCommand())
	rootCmd.AddCommand(newImportCommand())
	rootCmd.AddCommand(newCreateFileCommand())
	rootCmd.AddCommand(newProviderCommand())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getDefaultServer() string {
	if server := os.Getenv("LOOM_SERVER"); server != "" {
		return server
	}
	return "http://localhost:8080"
}

// --- HTTP client ---

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func newClient() *Client {
	return &Client{
		BaseURL: serverURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(method, path string, params url.Values, data interface{}) ([]byte, error) {
	u := fmt.Sprintf("%s%s", c.BaseURL, path)
	if params != nil {
		u += "?" + params.Encode()
	}

	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data: %w", err)
		}
		body = strings.NewReader(string(jsonData))
	}

	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) get(path string, params url.Values) ([]byte, error) {
	return c.do("GET", path, params, nil)
}

func (c *Client) post(path string, data interface{}) ([]byte, error) {
	return c.do("POST", path, nil, data)
}

func (c *Client) put(path string, data interface{}) ([]byte, error) {
	return c.do("PUT", path, nil, data)
}

func (c *Client) delete(path string) ([]byte, error) {
	return c.do("DELETE", path, nil, nil)
}

// streamSSE reads an SSE stream and prints each event's data field as JSON.
func (c *Client) streamSSE(path string) error {
	u := fmt.Sprintf("%s%s", c.BaseURL, path)
	resp, err := c.HTTP.Get(u)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			fmt.Println(line[6:])
		}
	}
	return scanner.Err()
}

// outputJSON prints data according to the global outputFormat flag.
func outputJSON(data []byte, filterApplied ...bool) {
	// Parse JSON first
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		// Not valid JSON, print raw
		fmt.Println(string(data))
		return
	}

	// Handle table format
	if outputFormat == "table" {
		filtered := false
		if len(filterApplied) > 0 {
			filtered = filterApplied[0]
		}
		if err := outputTable(v, filtered); err != nil {
			// Fallback to JSON if table formatting fails
			fmt.Fprintf(os.Stderr, "Warning: table formatting failed (%v), falling back to JSON\n", err)
			outputFormatJSON(v)
		}
		return
	}

	// Default to JSON
	outputFormatJSON(v)
}

// outputFormatJSON prints formatted JSON
func outputFormatJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

var columnPriority = map[string]int{
	"id": 0, "project_id": 1, "name": 2, "title": 2,
	"status": 3, "priority": 4, "assigned_to": 5, "type": 6,
	"created_at": 100, "updated_at": 101,
}

func sortColumns(columns []string, filterApplied bool) []string {
	if filterApplied {
		filtered := make([]string, 0, len(columns))
		for _, col := range columns {
			if col != "project_id" {
				filtered = append(filtered, col)
			}
		}
		columns = filtered
	}
	for i := 0; i < len(columns); i++ {
		for j := i + 1; j < len(columns); j++ {
			pri1 := columnPriority[columns[i]]
			if pri1 == 0 && columns[i] != "id" {
				pri1 = 50
			}
			pri2 := columnPriority[columns[j]]
			if pri2 == 0 && columns[j] != "id" {
				pri2 = 50
			}
			if pri2 < pri1 {
				columns[i], columns[j] = columns[j], columns[i]
			}
		}
	}
	return columns
}

// outputTable formats data as a table
func outputTable(v interface{}, filterApplied bool) error {
	// Handle array of objects (most common case for list commands)
	arr, ok := v.([]interface{})
	if !ok {
		// Single object or non-array - show as key-value pairs
		return outputTableObject(v)
	}

	if len(arr) == 0 {
		fmt.Println("(no results)")
		return nil
	}

	// Extract all possible columns from all objects
	columnSet := make(map[string]bool)
	var columns []string

	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for key := range obj {
			if !columnSet[key] {
				columnSet[key] = true
				columns = append(columns, key)
			}
		}
	}

	// Sort and filter columns
	columns = sortColumns(columns, filterApplied)

	if len(columns) == 0 {
		return fmt.Errorf("no columns found")
	}

	// Print header
	fmt.Print(columns[0])
	for _, col := range columns[1:] {
		fmt.Printf("\t%s", col)
	}
	fmt.Println()

	// Print separator
	for i := 0; i < len(columns); i++ {
		if i > 0 {
			fmt.Print("\t")
		}
		fmt.Print("---")
	}
	fmt.Println()

	// Print rows
	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		fmt.Print(formatValue(obj[columns[0]]))
		for _, col := range columns[1:] {
			fmt.Printf("\t%s", formatValue(obj[col]))
		}
		fmt.Println()
	}

	return nil
}

// outputTableObject formats a single object as key-value pairs
func outputTableObject(v interface{}) error {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected object, got %T", v)
	}

	fmt.Println("KEY\tVALUE")
	fmt.Println("---\t-----")

	for key, val := range obj {
		fmt.Printf("%s\t%s\n", key, formatValue(val))
	}

	return nil
}

// formatValue converts a JSON value to a string for table display
func formatValue(v interface{}) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		// Truncate long strings
		if len(val) > 50 {
			return val[:47] + "..."
		}
		return val
	case float64:
		// Remove decimal if it's a whole number
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%.2f", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case map[string]interface{}:
		// Nested object - show as placeholder
		return "(object)"
	case []interface{}:
		// Array - show count
		return fmt.Sprintf("(array:%d)", len(val))
	default:
		return fmt.Sprintf("%v", val)
	}
}

// --- Bead commands ---

func newBeadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bead",
		Short: "Manage beads (work items)",
	}
	cmd.AddCommand(newBeadListCommand())
	cmd.AddCommand(newBeadCreateCommand())
	cmd.AddCommand(newBeadShowCommand())
	cmd.AddCommand(newBeadClaimCommand())
	cmd.AddCommand(newBeadPokeCommand())
	cmd.AddCommand(newBeadUpdateCommand())
	cmd.AddCommand(newBeadBulkUpdateCommand())
	cmd.AddCommand(newBeadDeleteCommand())
	cmd.AddCommand(newBeadErrorsCommand())
	cmd.AddCommand(newBeadUnblockCommand())
	return cmd
}

func newBeadListCommand() *cobra.Command {
	var (
		projectID   string
		status      string
		beadType    string
		assignedTo  string
		priority    int
		hasPriority bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List beads",
		Example: `  loomctl bead list
  loomctl bead list --status=open --project=loom
  loomctl bead list --priority=0 --status=open`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if status != "" {
				params.Set("status", status)
				if hasPriority {
					params.Set("priority", fmt.Sprintf("%d", priority))
				}
				if assignedTo != "" {
					params.Set("assigned_to", assignedTo)
				}
			}
			if beadType != "" {
				params.Set("type", beadType)
			}
			if assignedTo != "" {
				params.Set("assigned_to", assignedTo)
			}
			if hasPriority && priority >= 0 {
				params.Set("priority", fmt.Sprintf("%d", priority))
			}
			data, err := client.get("/api/v1/beads", params)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (open, in_progress, closed, blocked)")
	cmd.Flags().StringVar(&beadType, "type", "", "Filter by bead type (task, bug, feature)")
	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "Filter by assigned agent")
	cmd.Flags().IntVarP(&priority, "priority", "P", 0, "Filter by priority (0=P0/highest, 4=lowest)")

	cmd.Flags().BoolVar(&hasPriority, "has-priority", false, "")
	cmd.Flags().MarkHidden("has-priority")
	// Use PreRunE to detect if --priority was explicitly set
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		hasPriority = c.Flags().Changed("priority")
		return nil
	}
	return cmd
}

func newBeadCreateCommand() *cobra.Command {
	var (
		title       string
		description string
		priority    int
		projectID   string
		beadType    string
	)
	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a new bead",
		Example: `  loomctl bead create --title="Fix bug" --project=loom`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			body := map[string]interface{}{
				"title":       title,
				"description": description,
				"priority":    priority,
				"project_id":  projectID,
			}
			if beadType != "" {
				body["type"] = beadType
			}
			data, err := client.post("/api/v1/beads", body)
			if err != nil {
				return err
			}
			outputJSON(data, projectID != "")
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Bead title (required)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Bead description")
	cmd.Flags().IntVar(&priority, "priority", 2, "Priority (0=highest, 4=lowest)")
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (required)")
	cmd.Flags().StringVar(&beadType, "type", "task", "Bead type")
	cmd.MarkFlagRequired("title")
	cmd.MarkFlagRequired("project")
	return cmd
}

func newBeadShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "show <bead-id>",
		Short:   "Show bead details",
		Args:    cobra.ExactArgs(1),
		Example: `  loomctl bead show loom-001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get(fmt.Sprintf("/api/v1/beads/%s", args[0]), nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newBeadClaimCommand() *cobra.Command {
	var agentID string
	cmd := &cobra.Command{
		Use:   "claim <bead-id>",
		Short: "Claim a bead for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.post(fmt.Sprintf("/api/v1/beads/%s/claim", args[0]), map[string]interface{}{"agent_id": agentID})
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&agentID, "agent", "a", "", "Agent ID (required)")
	cmd.MarkFlagRequired("agent")
	return cmd
}

func newBeadPokeCommand() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:     "poke <bead-id>",
		Short:   "Redispatch a stuck bead",
		Args:    cobra.ExactArgs(1),
		Example: `  loomctl bead poke loom-001 --reason="fixes deployed"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			body := map[string]interface{}{}
			if reason != "" {
				body["reason"] = reason
			}
			data, err := client.post(fmt.Sprintf("/api/v1/beads/%s/redispatch", args[0]), body)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "Reason for redispatch")
	return cmd
}

func newBeadUpdateCommand() *cobra.Command {
	var (
		status   string
		priority int
		title    string
	)
	cmd := &cobra.Command{
		Use:   "update <bead-id>",
		Short: "Update bead fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			body := map[string]interface{}{}
			if cmd.Flags().Changed("status") {
				body["status"] = status
			}
			if cmd.Flags().Changed("priority") {
				body["priority"] = priority
			}
			if cmd.Flags().Changed("title") {
				body["title"] = title
			}
			data, err := client.put(fmt.Sprintf("/api/v1/beads/%s", args[0]), body)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "New status")
	cmd.Flags().IntVar(&priority, "priority", 0, "New priority")
	cmd.Flags().StringVar(&title, "title", "", "New title")
	return cmd
}

func newBeadDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <bead-id>",
		Short: "Delete a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.delete(fmt.Sprintf("/api/v1/beads/%s", args[0]))
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newBeadErrorsCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "errors <bead-id>",
		Short:   "Show error history for a bead",
		Args:    cobra.ExactArgs(1),
		Example: `  loomctl bead errors loom-001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get(fmt.Sprintf("/api/v1/beads/%s", args[0]), nil)
			if err != nil {
				return err
			}
			var bead map[string]interface{}
			if err := json.Unmarshal(data, &bead); err != nil {
				return err
			}
			ctx, _ := bead["context"].(map[string]interface{})
			if ctx == nil {
				fmt.Println("(no context)")
				return nil
			}

			// Print key diagnostic fields.
			fmt.Printf("Bead:          %s\n", args[0])
			fmt.Printf("Status:        %v\n", bead["status"])
			if dc, ok := ctx["dispatch_count"]; ok {
				fmt.Printf("Dispatches:    %v\n", dc)
			}
			if lre, ok := ctx["last_run_error"]; ok && lre != "" {
				fmt.Printf("Last error:    %v\n", lre)
			}
			if rr, ok := ctx["ralph_blocked_reason"]; ok && rr != "" {
				fmt.Printf("Blocked:       %v\n", rr)
				fmt.Printf("Blocked at:    %v\n", ctx["ralph_blocked_at"])
			}
			if ld, ok := ctx["loop_detected"]; ok && ld == "true" {
				fmt.Printf("Loop reason:   %v\n", ctx["loop_detected_reason"])
			}

			// Parse and display error_history JSON array.
			histJSON, _ := ctx["error_history"].(string)
			if histJSON == "" {
				fmt.Println("\nNo error history recorded.")
				return nil
			}
			var history []map[string]interface{}
			if err := json.Unmarshal([]byte(histJSON), &history); err != nil {
				fmt.Printf("\nRaw error_history: %s\n", histJSON)
				return nil
			}
			fmt.Printf("\nError history (%d entries):\n", len(history))
			for i, entry := range history {
				ts, _ := entry["timestamp"].(string)
				errMsg, _ := entry["error"].(string)
				dc, _ := entry["dispatch"].(float64)
				errMsg = strings.TrimSpace(errMsg)
				if len(errMsg) > 100 {
					errMsg = errMsg[:97] + "..."
				}
				fmt.Printf("  [%2d] dispatch=%-3.0f  %s\n       %s\n", i+1, dc, ts, errMsg)
			}
			return nil
		},
	}
}

func newBeadUnblockCommand() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "unblock <bead-id>",
		Short: "Unblock a blocked bead and reset its error state",
		Long: `Clears ralph_blocked_reason, error_history, and dispatch_count, then
re-opens the bead so the task executor will pick it up again.`,
		Args:    cobra.ExactArgs(1),
		Example: `  loomctl bead unblock loom-001 --reason="provider fixed"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			body := map[string]interface{}{
				"reason": reason,
			}
			if reason == "" {
				body["reason"] = "manually unblocked via loomctl"
			}
			data, err := client.post(fmt.Sprintf("/api/v1/beads/%s/redispatch", args[0]), body)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "Reason for unblocking")
	return cmd
}

func newBeadBulkUpdateCommand() *cobra.Command {
	var (
		status   string
		priority int
		title    string
	)
	cmd := &cobra.Command{
		Use:   "bulk-update <bead-ids>",
		Short: "Bulk update bead fields",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			body := map[string]interface{}{}
			if cmd.Flags().Changed("status") {
				body["status"] = status
			}
			if cmd.Flags().Changed("priority") {
				body["priority"] = priority
			}
			if cmd.Flags().Changed("title") {
				body["title"] = title
			}
			for _, beadID := range args {
				data, err := client.put(fmt.Sprintf("/api/v1/beads/%s", beadID), body)
				if err != nil {
					return err
				}
				outputJSON(data)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "New status")
	cmd.Flags().IntVar(&priority, "priority", 0, "New priority")
	cmd.Flags().StringVar(&title, "title", "", "New title")
	return cmd
}

// --- Log commands ---

func newLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "View and stream logs",
	}
	cmd.AddCommand(newLogRecentCommand())
	cmd.AddCommand(newLogStreamCommand())
	cmd.AddCommand(newLogExportCommand())
	return cmd
}

func newLogRecentCommand() *cobra.Command {
	var (
		limit     int
		projectID string
		source    string
		level     string
		agentID   string
		beadID    string
		since     string
	)
	cmd := &cobra.Command{
		Use:     "recent",
		Short:   "Show recent log entries",
		Example: `  loomctl log recent --limit=50 --project=loom --level=error --since=1h`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if limit > 0 {
				params.Set("limit", fmt.Sprintf("%d", limit))
			}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if source != "" {
				params.Set("source", source)
			}
			if level != "" {
				params.Set("level", level)
			}
			if agentID != "" {
				params.Set("agent_id", agentID)
			}
			if beadID != "" {
				params.Set("bead_id", beadID)
			}
			if since != "" {
				params.Set("since", since)
			}
			data, err := client.get("/api/v1/logs/recent", params)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 100, "Number of log entries")
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID")
	cmd.Flags().StringVar(&source, "source", "", "Filter by log source")
	cmd.Flags().StringVar(&level, "level", "", "Filter by log level (info, warn, error)")
	cmd.Flags().StringVar(&agentID, "agent", "", "Filter by agent ID")
	cmd.Flags().StringVar(&beadID, "bead", "", "Filter by bead ID")
	cmd.Flags().StringVar(&since, "since", "", "Show logs since timestamp (RFC3339) or duration (e.g. 1h, 30m)")
	return cmd
}

func newLogStreamCommand() *cobra.Command {
	var (
		projectID string
		source    string
		level     string
	)
	cmd := &cobra.Command{
		Use:   "stream",
		Short: "Stream live logs (SSE)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if source != "" {
				params.Set("source", source)
			}
			if level != "" {
				params.Set("level", level)
			}
			u := "/api/v1/logs/stream"
			if len(params) > 0 {
				u += "?" + params.Encode()
			}
			return client.streamSSE(u)
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID")
	cmd.Flags().StringVar(&source, "source", "", "Filter by log source")
	cmd.Flags().StringVar(&level, "level", "", "Filter by log level")
	return cmd
}

func newLogExportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export all logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/logs/export", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

// --- Status command ---

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show system status overview",
		Long:  "Aggregates health, agents, and bead counts into one JSON object",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			result := map[string]interface{}{}

			// Health
			if data, err := client.get("/api/v1/health", nil); err == nil {
				var v interface{}
				if json.Unmarshal(data, &v) == nil {
					result["health"] = v
				}
			}

			// Agents
			if data, err := client.get("/api/v1/agents", nil); err == nil {
				var agents []interface{}
				if json.Unmarshal(data, &agents) == nil {
					working, idle := 0, 0
					for _, a := range agents {
						am, _ := a.(map[string]interface{})
						if am["status"] == "working" {
							working++
						} else if am["status"] == "idle" {
							idle++
						}
					}
					result["agents"] = map[string]interface{}{
						"total":   len(agents),
						"working": working,
						"idle":    idle,
					}
				}
			}

			// Beads
			if data, err := client.get("/api/v1/beads", nil); err == nil {
				var beads []interface{}
				if json.Unmarshal(data, &beads) == nil {
					counts := map[string]int{}
					for _, b := range beads {
						bm, _ := b.(map[string]interface{})
						s, _ := bm["status"].(string)
						counts[s]++
					}
					result["beads"] = map[string]interface{}{
						"total":     len(beads),
						"by_status": counts,
					}
				}
			}

			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
	return cmd
}

// --- Metrics / Observability commands ---

func newMetricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Retrieve observability metrics",
	}
	cmd.AddCommand(newMetricsPrometheusCommand())
	cmd.AddCommand(newMetricsCacheCommand())
	cmd.AddCommand(newMetricsPatternsCommand())
	cmd.AddCommand(newMetricsEventsStatsCommand())
	return cmd
}

func newMetricsPrometheusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prometheus",
		Short: "Fetch raw Prometheus metrics from the /metrics endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/metrics", nil)
			if err != nil {
				return err
			}
			// Prometheus metrics are text, wrap in JSON
			out, _ := json.MarshalIndent(map[string]interface{}{
				"format": "prometheus",
				"raw":    string(data),
			}, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

func newMetricsCacheCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cache",
		Short: "Show cache statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/cache/stats", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newMetricsPatternsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "patterns",
		Short: "Show pattern analysis results",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/patterns/analysis", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newMetricsEventsStatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "events",
		Short: "Show event statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/events/stats", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

// --- Conversation commands ---

func newConversationCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "conversation",
		Aliases: []string{"conv"},
		Short:   "View agent conversations",
	}
	cmd.AddCommand(newConversationListCommand())
	cmd.AddCommand(newConversationShowCommand())
	return cmd
}

func newConversationListCommand() *cobra.Command {
	var (
		projectID string
		limit     int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List conversation sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if limit > 0 {
				params.Set("limit", fmt.Sprintf("%d", limit))
			}
			data, err := client.get("/api/v1/conversations", params)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID (required by server)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "Number of conversations")
	return cmd
}

func newConversationShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show conversation messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get(fmt.Sprintf("/api/v1/conversations/%s", args[0]), nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

// --- Analytics commands ---

func newAnalyticsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analytics",
		Short: "View analytics and cost data",
	}
	cmd.AddCommand(newAnalyticsStatsCommand())
	cmd.AddCommand(newAnalyticsCostsCommand())
	cmd.AddCommand(newAnalyticsLogsCommand())
	cmd.AddCommand(newAnalyticsExportCommand())
	cmd.AddCommand(newAnalyticsVelocityCommand())
	return cmd
}

func newAnalyticsStatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show analytics statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/analytics/stats", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newAnalyticsCostsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "costs",
		Short: "Show cost breakdown by provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/analytics/costs", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newAnalyticsLogsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logs",
		Short: "Show analytics log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/analytics/logs", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newAnalyticsExportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export analytics data",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/analytics/export", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newAnalyticsVelocityCommand() *cobra.Command {
	var projectID string
	var timeWindow string

	cmd := &cobra.Command{
		Use:   "velocity",
		Short: "Show change velocity metrics (commits, pushes, builds, tests)",
		Long: `Display git change velocity and development funnel metrics for a project.
Shows files modified, builds/tests run, commits made, and pushes completed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if timeWindow != "" {
				params.Set("time_window", timeWindow)
			}

			data, err := client.get("/api/v1/analytics/change-velocity", params)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID to query (required)")
	cmd.Flags().StringVarP(&timeWindow, "window", "w", "24h", "Time window (e.g., 24h, 7d, 30d)")

	return cmd
}

// --- Config commands ---

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and manage server configuration",
	}
	cmd.AddCommand(newConfigShowCommand())
	cmd.AddCommand(newConfigExportCommand())
	return cmd
}

func newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current server configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/config", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newConfigExportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export configuration as YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/config/export.yaml", nil)
			if err != nil {
				return err
			}
			// YAML content, wrap in JSON
			out, _ := json.MarshalIndent(map[string]interface{}{
				"format":  "yaml",
				"content": string(data),
			}, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

// --- Event commands ---

func newEventCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "View events and activity feed",
	}
	cmd.AddCommand(newEventListCommand())
	cmd.AddCommand(newEventStreamCommand())
	cmd.AddCommand(newEventActivityCommand())
	return cmd
}

func newEventListCommand() *cobra.Command {
	var (
		projectID string
		eventType string
		limit     int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent events",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if eventType != "" {
				params.Set("type", eventType)
			}
			if limit > 0 {
				params.Set("limit", fmt.Sprintf("%d", limit))
			}
			data, err := client.get("/api/v1/events", params)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID")
	cmd.Flags().StringVar(&eventType, "type", "", "Filter by event type")
	cmd.Flags().IntVarP(&limit, "limit", "n", 100, "Number of events")
	return cmd
}

func newEventStreamCommand() *cobra.Command {
	var (
		projectID string
		eventType string
	)
	cmd := &cobra.Command{
		Use:   "stream",
		Short: "Stream events in real-time (SSE)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if eventType != "" {
				params.Set("type", eventType)
			}
			u := "/api/v1/events/stream"
			if len(params) > 0 {
				u += "?" + params.Encode()
			}
			return client.streamSSE(u)
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID")
	cmd.Flags().StringVar(&eventType, "type", "", "Filter by event type")
	return cmd
}

func newEventActivityCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "activity",
		Short: "Show activity feed",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/activity-feed", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

// --- Workflow commands ---

func newWorkflowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage workflows",
	}
	cmd.AddCommand(newWorkflowListCommand())
	cmd.AddCommand(newWorkflowShowCommand())
	cmd.AddCommand(newWorkflowStartCommand())
	cmd.AddCommand(newWorkflowExecutionsCommand())
	cmd.AddCommand(newWorkflowAnalyticsCommand())
	return cmd
}

func newWorkflowListCommand() *cobra.Command {
	var (
		projectID    string
		workflowType string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			if workflowType != "" {
				params.Set("type", workflowType)
			}
			data, err := client.get("/api/v1/workflows", params)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID")
	cmd.Flags().StringVar(&workflowType, "type", "", "Filter by workflow type")
	return cmd
}

func newWorkflowShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <workflow-id>",
		Short: "Show workflow details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get(fmt.Sprintf("/api/v1/workflows/%s", args[0]), nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newWorkflowStartCommand() *cobra.Command {
	var (
		workflowID string
		beadID     string
		projectID  string
	)
	cmd := &cobra.Command{
		Use:     "start",
		Short:   "Start a workflow execution",
		Example: `  loomctl workflow start --workflow=wf-ui-default --bead=loom-001 --project=loom`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.post("/api/v1/workflows/start", map[string]interface{}{
				"workflow_id": workflowID,
				"bead_id":     beadID,
				"project_id":  projectID,
			})
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&workflowID, "workflow", "w", "", "Workflow ID (required)")
	cmd.Flags().StringVarP(&beadID, "bead", "b", "", "Bead ID (required)")
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (required)")
	cmd.MarkFlagRequired("workflow")
	cmd.MarkFlagRequired("bead")
	cmd.MarkFlagRequired("project")
	return cmd
}

func newWorkflowExecutionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "executions",
		Short: "List workflow executions",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/workflows/executions", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newWorkflowAnalyticsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "analytics",
		Short: "Show workflow analytics",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/workflows/analytics", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

// --- Agent commands ---

func newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents",
	}
	cmd.AddCommand(newAgentListCommand())
	cmd.AddCommand(newAgentShowCommand())
	return cmd
}

func newAgentListCommand() *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			params := url.Values{}
			if projectID != "" {
				params.Set("project_id", projectID)
			}
			data, err := client.get("/api/v1/agents", params)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Filter by project ID")
	return cmd
}

func newAgentShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <agent-id>",
		Short: "Show agent details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get(fmt.Sprintf("/api/v1/agents/%s", args[0]), nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

// --- Project commands ---

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}
	cmd.AddCommand(newProjectListCommand())
	cmd.AddCommand(newProjectShowCommand())
	cmd.AddCommand(newProjectResetBeadsCommand())
	return cmd
}

func newProjectResetBeadsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-beads <project-id>",
		Short: "Reload a project's beads from its beads-sync branch",
		Long: `Clears the server's in-memory bead state for a project and reloads it
from the beads-sync branch on disk. Use this after a force-push, dolt
recovery, or any out-of-band change to the project's beads worktree.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.post(fmt.Sprintf("/api/v1/projects/%s/beads/reset", args[0]), nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newProjectListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/projects", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newProjectShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <project-id>",
		Short: "Show project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get(fmt.Sprintf("/api/v1/projects/%s", args[0]), nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

// --- Provider commands ---

func newProviderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage LLM providers",
	}
	cmd.AddCommand(newProviderListCommand())
	cmd.AddCommand(newProviderShowCommand())
	cmd.AddCommand(newProviderRegisterCommand())
	cmd.AddCommand(newProviderDeleteCommand())
	return cmd
}

func newProviderListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get("/api/v1/providers", nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newProviderShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <provider-id>",
		Short: "Show provider details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.get(fmt.Sprintf("/api/v1/providers/%s", args[0]), nil)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}

func newProviderRegisterCommand() *cobra.Command {
	var (
		name        string
		provType    string
		endpoint    string
		model       string
		apiKey      string
		description string
	)
	cmd := &cobra.Command{
		Use:   "register <id>",
		Short: "Register or update a provider",
		Args:  cobra.ExactArgs(1),
		Example: `  loomctl provider register tokenhub --name="TokenHub" --type=openai \
    --endpoint="http://172.17.0.1:8090/v1" --model="nvidia/openai/gpt-oss-20b" --api-key="sk-..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			body := map[string]interface{}{
				"id":       args[0],
				"name":     name,
				"type":     provType,
				"endpoint": endpoint,
				"model":    model,
			}
			if apiKey != "" {
				body["api_key"] = apiKey
			}
			if description != "" {
				body["description"] = description
			}
			data, err := client.post("/api/v1/providers", body)
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Display name")
	cmd.Flags().StringVar(&provType, "type", "openai", "Provider type (openai, anthropic)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "API endpoint URL")
	cmd.Flags().StringVar(&model, "model", "", "Default model ID")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	return cmd
}

func newProviderDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <provider-id>",
		Short: "Delete a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			data, err := client.delete(fmt.Sprintf("/api/v1/providers/%s", args[0]))
			if err != nil {
				return err
			}
			outputJSON(data)
			return nil
		},
	}
}
