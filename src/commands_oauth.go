package app

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func cmdOAuth(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lisa oauth <subcommand>")
		return 1
	}
	if args[0] == "--help" || args[0] == "-h" {
		return showHelp("oauth")
	}
	if args[0] == "help" {
		if len(args) > 1 {
			return showHelp("oauth " + args[1])
		}
		return showHelp("oauth")
	}

	switch args[0] {
	case "add":
		return cmdOAuthAdd(args[1:])
	case "list":
		return cmdOAuthList(args[1:])
	case "remove":
		return cmdOAuthRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown oauth subcommand: %s\n", args[0])
		return 1
	}
}

func cmdOAuthAdd(args []string) int {
	token := ""
	tokenFromStdin := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("oauth add")
		case "--token":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --token")
			}
			token = args[i+1]
			i++
		case "--stdin":
			tokenFromStdin = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if tokenFromStdin && strings.TrimSpace(token) != "" {
		return commandError(jsonOut, "invalid_oauth_input", "--token and --stdin are mutually exclusive")
	}
	if tokenFromStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return commandErrorf(jsonOut, "oauth_read_failed", "failed to read oauth token from stdin: %v", err)
		}
		token = string(raw)
	}
	if strings.TrimSpace(token) == "" {
		return commandError(jsonOut, "missing_required_flag", "oauth token required (use --token or --stdin)")
	}

	record, added, err := addClaudeOAuthToken(token)
	if err != nil {
		return commandErrorf(jsonOut, "oauth_add_failed", "failed adding oauth token: %v", err)
	}
	count, err := claudeOAuthTokenCount()
	if err != nil {
		return commandErrorf(jsonOut, "oauth_count_failed", "failed reading oauth token count: %v", err)
	}

	if jsonOut {
		writeJSON(map[string]any{
			"id":    record.ID,
			"added": added,
			"count": count,
		})
		return 0
	}

	if added {
		fmt.Printf("oauth token added: %s\n", record.ID)
	} else {
		fmt.Printf("oauth token already present: %s\n", record.ID)
	}
	return 0
}

func cmdOAuthList(args []string) int {
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("oauth list")
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	rows, err := listClaudeOAuthTokens()
	if err != nil {
		return commandErrorf(jsonOut, "oauth_list_failed", "failed listing oauth tokens: %v", err)
	}

	if jsonOut {
		writeJSON(map[string]any{
			"count":  len(rows),
			"tokens": rows,
		})
		return 0
	}
	if len(rows) == 0 {
		fmt.Println("no oauth tokens configured")
		return 0
	}
	for _, row := range rows {
		next := ""
		if row.Next {
			next = "next"
		}
		if err := writeCSVRecord(row.ID, row.AddedAt, row.LastUsedAt, strconv.Itoa(row.UseCount), next); err != nil {
			return commandErrorf(false, "oauth_list_write_failed", "failed writing oauth token list: %v", err)
		}
	}
	return 0
}

func cmdOAuthRemove(args []string) int {
	id := ""
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("oauth remove")
		case "--id":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --id")
			}
			id = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	if strings.TrimSpace(id) == "" {
		return commandError(jsonOut, "missing_required_flag", "--id is required")
	}

	removed, err := removeClaudeOAuthTokenByID(id, "manual_remove")
	if err != nil {
		return commandErrorf(jsonOut, "oauth_remove_failed", "failed removing oauth token: %v", err)
	}
	if !removed {
		return commandErrorf(jsonOut, "oauth_token_not_found", "oauth token not found: %s", id)
	}

	if jsonOut {
		writeJSON(map[string]any{
			"id":      strings.TrimSpace(id),
			"removed": true,
		})
		return 0
	}
	fmt.Printf("oauth token removed: %s\n", strings.TrimSpace(id))
	return 0
}
