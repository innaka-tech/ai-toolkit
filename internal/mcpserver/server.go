// Package mcpserver exposes aitk over the Model Context Protocol (docs/spec/integrations.md).
// Every tool runs the same code path as the CLI (in-process, with --json), so MCP and CLI
// effects are identical.
package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/innaka-tech/ai-toolkit/v2/internal/slash"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exec runs aitk CLI arguments and writes the JSON envelope to stdout.
type Exec func(args []string, stdout, stderr *bytes.Buffer) int

const instructions = `aitk keeps this project's tasks, handoffs, and knowledge in git and decides when work is done.
Use these tools on your own, without waiting to be asked:
1) At the start of every session, call brief first and follow it.
2) Continue the active task, or call task_next with start=true (task_new if nothing fits).
3) Do the work; call check until it passes; call impact and cover what it says is untested.
4) Call close with a summary and one lasting finding as knowledge ("none" allowed). If close refuses, do what its fix says.
If unsure, blocked, the change is risky, or check fails twice: close with status blocked and ask the user.
Never accept user acceptance (UAT) for the user. If brief reports E_NOT_A_PROJECT, this repository does not use aitk: ignore these tools.
Errors include a "fix" with the exact next step.`

// New builds the server for the project at root.
func New(root, version string, exec Exec) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "aitk", Title: "aitk", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	h := &handler{root: root, exec: exec}
	add(s, h, "brief", "Call this first in every session, without being asked. The bounded session brief: active task, open criteria, last handoff, relevant knowledge, conventions, rules, next commands.",
		func(in briefIn) []string {
			a := []string{"brief"}
			if in.Budget > 0 {
				a = append(a, "--budget", strconv.Itoa(in.Budget))
			}
			if in.Task != "" {
				a = append(a, "--task", in.Task)
			}
			return a
		})
	add(s, h, "task_new", "Create a task. Give at least one acceptance criterion (Given/when/then) unless the change is trivial.",
		func(in taskNewIn) []string {
			a := []string{"task", "new"}
			a = opt(a, "--id", in.ID)
			a = opt(a, "--goal", in.Goal)
			a = opt(a, "--profile", in.Profile)
			a = opt(a, "--objective", in.Objective)
			a = multi(a, "--ac", in.Criteria)
			a = multi(a, "--tag", in.Tags)
			a = multi(a, "--depends", in.DependsOn)
			if in.Start {
				a = append(a, "--start")
			}
			return append(a, "--", in.Title)
		})
	add(s, h, "task_start", "Make a task active in this worktree (status in_progress).",
		func(in idIn) []string { return []string{"task", "start", "--", in.ID} })
	add(s, h, "task_update", "Edit task fields: mark criteria done, add criteria, add a note, change status or tags. Defaults to the active task.",
		func(in taskUpdateIn) []string {
			a := []string{"task", "update"}
			a = opt(a, "--status", in.Status)
			a = opt(a, "--note", in.Note)
			a = opt(a, "--profile", in.Profile)
			if len(in.ACDone) > 0 {
				var ns []string
				for _, n := range in.ACDone {
					ns = append(ns, strconv.Itoa(n))
				}
				a = append(a, "--ac-done", strings.Join(ns, ","))
			}
			a = multi(a, "--add-ac", in.AddAC)
			a = multi(a, "--tag", in.Tags)
			if in.ID != "" {
				a = append(a, "--", in.ID)
			}
			return a
		})
	add(s, h, "task_list", "List tasks (open ones unless all is true).",
		func(in taskListIn) []string {
			a := []string{"task", "list"}
			a = opt(a, "--status", in.Status)
			if in.All {
				a = append(a, "--all")
			}
			return a
		})
	add(s, h, "task_show", "Show one task with its criteria.", func(in idIn) []string { return []string{"task", "show", "--", in.ID} })
	add(s, h, "check", "Run the project's check command; records evidence on the active task. Must pass before close.",
		func(struct{}) []string { return []string{"check"} })
	add(s, h, "close", "Finish the session. Applies the Definition of Done; on rejection the error says exactly what to do.",
		func(in closeIn) []string {
			a := []string{"close", "--summary", in.Summary, "--knowledge", in.Knowledge}
			a = multi(a, "--next", in.Next)
			a = multi(a, "--tag", in.Tags)
			a = opt(a, "--status", in.Status)
			return a
		})
	add(s, h, "review_pass", "Record one bug-hunt review pass on the active task (strict tasks need two, the last with 0 findings).",
		func(in reviewIn) []string { return []string{"review", "pass", "--findings", strconv.Itoa(in.Findings)} })
	add(s, h, "knowledge_search", "Search project knowledge.",
		func(in searchIn) []string {
			a := []string{"knowledge", "search"}
			if in.Limit > 0 {
				a = append(a, "--limit", strconv.Itoa(in.Limit))
			}
			return append(a, "--", in.Query)
		})
	add(s, h, "knowledge_add", "Record a lasting finding (one or two sentences).",
		func(in knowledgeIn) []string {
			a := multi([]string{"knowledge", "add"}, "--tag", in.Tags)
			if in.Pin {
				a = append(a, "--pin")
			}
			return append(a, "--", in.Text)
		})
	add(s, h, "switch_prepare", "Hand the work to another AI tool: writes a handoff and returns the prompt file and command that continue the work there.",
		func(in switchIn) []string {
			a := opt([]string{"switch", "--print"}, "--note", in.Note)
			return append(a, "--", in.Tool)
		})
	add(s, h, "report", "Activity report: done, in progress, in review, blocked tasks and handoffs per tool.",
		func(in reportIn) []string {
			a := []string{"report"}
			return opt(a, "--since", in.Since)
		})
	add(s, h, "audit", "Security audit (dependency vulnerabilities, static analysis, secrets). Required before close for strict tasks by default. With tasks=true, findings become fix tasks.",
		func(in auditIn) []string {
			if in.Tasks {
				return []string{"audit", "--tasks"}
			}
			return []string{"audit"}
		})
	add(s, h, "task_next", "Tasks that can be worked on now, in order (dependencies done, not claimed by another worktree). With start=true, starts the first one.",
		func(in taskNextIn) []string {
			a := []string{"task", "next"}
			a = opt(a, "--goal", in.Goal)
			a = opt(a, "--tag", in.Tag)
			if in.Start {
				a = append(a, "--start")
			}
			return a
		})
	add(s, h, "impact", "Blast radius of the current change: files that reference each changed file and the tests that cover it. Run before close; add tests for files no test covers.",
		func(struct{}) []string { return []string{"impact"} })
	add(s, h, "security_checklist", "Add the OWASP ASVS checklist to a task (strict tasks need every item verified or marked N/A with a reason).",
		func(in optIDIn) []string { return withID([]string{"security", "checklist"}, in.ID) })
	add(s, h, "uat_script", "Write a user acceptance test script for a person. Only the user can accept or reject (aitk uat accept|reject in their terminal).",
		func(in optIDIn) []string { return withID([]string{"uat", "script"}, in.ID) })
	add(s, h, "adr_new", "Create an architecture decision record (MADR).", func(in titleIn) []string { return []string{"adr", "new", "--", in.Title} })
	add(s, h, "doctor", "Validate the project; fix=true repairs what is safe.",
		func(in doctorIn) []string {
			if in.Fix {
				return []string{"doctor", "--fix"}
			}
			return []string{"doctor"}
		})

	s.AddResource(&mcp.Resource{URI: "aitk://brief", Name: "brief", Title: "Session brief", MIMEType: "text/markdown"},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			text, _, _ := h.call(req.Session, []string{"brief"})
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "aitk://brief", MIMEType: "text/markdown", Text: text}}}, nil
		})
	s.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "aitk://task/{id}", Name: "task", Title: "Task (aitk.result/v1 JSON)", MIMEType: "application/json"},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			id := strings.TrimPrefix(req.Params.URI, "aitk://task/")
			if id == "" || strings.ContainsAny(id, "/\\") {
				return nil, mcp.ResourceNotFoundError(req.Params.URI)
			}
			text, _, ok := h.call(req.Session, []string{"task", "show", "--", id})
			if !ok {
				return nil, mcp.ResourceNotFoundError(req.Params.URI)
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: text}}}, nil
		})
	// Slash commands and personas (built-in and the project's docs/ai/commands) as MCP prompts,
	// so any MCP client offers them; start-session and close-session stay as aliases.
	cmds, _ := slash.All(root)
	type alias struct{ name, of string }
	for _, a := range []alias{{"start-session", "aitk-start"}, {"close-session", "aitk-close"}} {
		for _, c := range cmds {
			if c.Name == a.of {
				c.Name, c.Description = a.name, c.Description+" (alias of /"+a.of+")"
				cmds = append(cmds, c)
				break
			}
		}
	}
	for _, c := range cmds {
		c := c
		s.AddPrompt(&mcp.Prompt{Name: c.Name, Description: c.Description, Arguments: []*mcp.PromptArgument{{Name: "args", Description: firstNonEmpty(c.ArgHint, "optional text for the command")}}},
			func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return &mcp.GetPromptResult{Description: c.Description, Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: c.Expand(req.Params.Arguments["args"])}}}}, nil
			})
	}
	return s
}

// ServeStdio serves over stdin/stdout until the client disconnects.
func ServeStdio(ctx context.Context, s *mcp.Server) error {
	return s.Run(ctx, &mcp.IOTransport{Reader: tolerant(os.Stdin, out), Writer: out, MaxLineLength: maxLine + 1})
}

// out serialises writes to stdout from the SDK and from tolerant's error replies.
var out = &lockedWriter{w: os.Stdout}

type lockedWriter struct {
	mu   sync.Mutex
	w    io.Writer
	last time.Time
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = time.Now()
	if isResponse(b) {
		inflight.Add(-1)
	}
	return l.w.Write(b)
}

// isResponse reports whether b is the SDK's answer to a request (an id that is not null).
func isResponse(b []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimSpace(b), &m) != nil {
		return false
	}
	_, res := m["result"]
	_, er := m["error"]
	id, ok := m["id"]
	return (res || er) && ok && string(bytes.TrimSpace(id)) != "null"
}

func (l *lockedWriter) Close() error { return nil }

// tolerant passes valid JSON-RPC messages to the SDK and answers anything else itself (parse
// error -32700 for invalid JSON, invalid request -32600 for JSON that is not a JSON-RPC message,
// including batches), so one bad line from a client does not end the session. Lines above the
// size limit are skipped with an error. At end of input it waits until replies have settled
// before closing, so a client that writes its requests and closes stdin still gets answers.
func tolerant(in io.Reader, reply io.Writer) io.ReadCloser {
	pr, pw := io.Pipe()
	errReply := func(code int, msg string) {
		reply.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":null,"error":{"code":%d,"message":%q}}`+"\n", code, msg)))
	}
	go func() {
		br := bufio.NewReaderSize(in, 64*1024)
		for {
			line, tooLong, err := readLine(br, maxLine)
			if tooLong {
				errReply(-32600, "Invalid Request: message larger than 32 MiB")
			} else if t := bytes.TrimSpace(line); len(t) > 0 {
				switch {
				case !json.Valid(t):
					errReply(-32700, "Parse error: not valid JSON")
				case !isMessage(t):
					errReply(-32600, "Invalid Request: not a JSON-RPC 2.0 request or notification (batches and responses are not accepted)")
				default:
					if hasID(t) {
						inflight.Add(1)
					}
					if _, werr := pw.Write(append(append([]byte{}, t...), '\n')); werr != nil {
						return
					}
				}
			}
			if err != nil {
				settle(reply)
				pw.CloseWithError(nil)
				return
			}
		}
	}()
	return pr
}

const maxLine = 32 << 20

// readLine reads one line (without the newline) up to max bytes; longer lines are consumed and
// reported as tooLong. err is set at end of input (the last line may still carry data).
func readLine(br *bufio.Reader, max int) (line []byte, tooLong bool, err error) {
	for {
		chunk, isPrefix, e := br.ReadLine()
		if len(line)+len(chunk) > max {
			tooLong = true
		} else if !tooLong {
			line = append(line, chunk...)
		}
		if e != nil {
			return line, tooLong, e
		}
		if !isPrefix {
			return line, tooLong, nil
		}
	}
}

// isMessage reports whether b is one JSON-RPC 2.0 request, notification, or response object.
func isMessage(b []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	var v string
	if json.Unmarshal(m["jsonrpc"], &v) != nil || v != "2.0" {
		return false
	}
	if id, ok := m["id"]; ok {
		switch t := bytes.TrimSpace(id); {
		case len(t) == 0, t[0] == '{', t[0] == '[', string(t) == "true", string(t) == "false":
			return false
		}
	}
	if id, ok := m["id"]; ok {
		var f float64
		if json.Unmarshal(id, &f) == nil && f != float64(int64(f)) {
			return false // fractional ids would be truncated
		}
	}
	// aitk never sends requests to the client, so a client message must be a request or a
	// notification: a stray response would only end the session.
	method, ok := m["method"]
	if !ok {
		return false
	}
	var name string
	if json.Unmarshal(method, &name) != nil || name == "" {
		return false
	}
	_, err := jsonrpc.DecodeMessage(b)
	return err == nil
}

// inflight counts requests passed to the SDK that have not been answered yet.
var inflight atomic.Int64

func hasID(b []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	id, ok := m["id"]
	return ok && string(bytes.TrimSpace(id)) != "null"
}

// settle waits, at end of input, until every request read has been answered (at most 15
// minutes, longer than the default check timeout), so a client that writes its requests and
// closes stdin still gets every answer.
func settle(w io.Writer) {
	if w != io.Writer(out) {
		return // only the real stdio transport has an SDK answering behind it
	}
	deadline := time.Now().Add(15 * time.Minute)
	for inflight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // let the last write finish
}

// ServeHTTP serves streamable HTTP on addr.
func ServeHTTP(addr string, s *mcp.Server) error {
	return http.ListenAndServe(addr, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil))
}

type handler struct {
	root string
	exec Exec
	once sync.Once
	mu   sync.Mutex
}

// identify sets AITK_TOOL from the MCP client name once, unless the user set it.
func (h *handler) identify(ss *mcp.ServerSession) {
	h.once.Do(func() {
		if os.Getenv("AITK_TOOL") != "" || ss == nil || ss.InitializeParams() == nil || ss.InitializeParams().ClientInfo == nil {
			return
		}
		os.Setenv("AITK_TOOL", ToolName(ss.InitializeParams().ClientInfo.Name))
	})
}

// ToolName maps MCP client names to aitk tool identifiers.
func ToolName(client string) string {
	c := strings.ToLower(client)
	for _, m := range []struct{ sub, name string }{
		{"claude", "claude-code"}, {"codex", "codex"}, {"gemini", "gemini-cli"}, {"opencode", "opencode"},
		{"cursor", "cursor"}, {"kiro", "kiro"}, {"windsurf", "windsurf"}, {"copilot", "copilot"}, {"jcode", "jcode"},
	} {
		if strings.Contains(c, m.sub) {
			return m.name
		}
	}
	var b strings.Builder
	for _, r := range c {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteRune('-')
		}
	}
	if s := strings.Trim(b.String(), "-"); s != "" {
		return s
	}
	return "mcp-client"
}

// call runs the CLI and returns (text for the model, envelope, ok).
func (h *handler) call(ss *mcp.ServerSession, args []string) (string, map[string]any, bool) {
	h.identify(ss)
	h.mu.Lock() // one command at a time: commands read process env and the working tree
	defer h.mu.Unlock()
	var out, errb bytes.Buffer
	h.exec(append([]string{"--json", "-C", h.root}, args...), &out, &errb)
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		return "internal error: " + out.String() + errb.String(), nil, false
	}
	ok, _ := env["ok"].(bool)
	if !ok {
		e, _ := env["error"].(map[string]any)
		text := fmt.Sprintf("error[%v]: %v", e["code"], e["message"])
		if fix, _ := e["fix"].(string); fix != "" {
			text += "\nfix: " + fix
		}
		return text, env, false
	}
	data := env["data"]
	if args[0] == "brief" {
		if m, _ := data.(map[string]any); m != nil {
			if md, _ := m["markdown"].(string); md != "" {
				return md, env, true
			}
		}
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	text := string(b)
	if ws, _ := env["warnings"].([]any); len(ws) > 0 {
		text += "\nwarnings:"
		for _, w := range ws {
			text += fmt.Sprintf("\n- %v", w)
		}
	}
	return text, env, true
}

func add[In any](s *mcp.Server, h *handler, name, desc string, argv func(In) []string) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		text, env, ok := h.call(req.Session, argv(in))
		res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: !ok}
		if env != nil {
			res.StructuredContent = env
		}
		return res, nil, nil
	})
}

func opt(a []string, flag, v string) []string {
	if v != "" {
		return append(a, flag, v)
	}
	return a
}

func multi(a []string, flag string, vs []string) []string {
	for _, v := range vs {
		a = append(a, flag, v)
	}
	return a
}

// Tool inputs. Field descriptions become the JSON Schema shown to the model.
type briefIn struct {
	Budget int    `json:"budget,omitempty" jsonschema:"token budget (default 4000)"`
	Task   string `json:"task,omitempty" jsonschema:"brief for this task id instead of the active one"`
}
type taskNewIn struct {
	Title     string   `json:"title" jsonschema:"short task title"`
	Criteria  []string `json:"criteria,omitempty" jsonschema:"acceptance criteria, e.g. Given ..., when ..., then ..."`
	Tags      []string `json:"tags,omitempty" jsonschema:"tags"`
	ID        string   `json:"id,omitempty" jsonschema:"custom id (default: random T-xxxx)"`
	Goal      string   `json:"goal,omitempty" jsonschema:"goal id G-..."`
	Profile   string   `json:"profile,omitempty" jsonschema:"pin risk profile: lite, standard, or strict"`
	Objective string   `json:"objective,omitempty" jsonschema:"one-paragraph objective"`
	Start     bool     `json:"start,omitempty" jsonschema:"start the task immediately"`
	DependsOn []string `json:"depends_on,omitempty" jsonschema:"ids of tasks that must be done first"`
}
type idIn struct {
	ID string `json:"id" jsonschema:"task id"`
}
type taskUpdateIn struct {
	ID      string   `json:"id,omitempty" jsonschema:"task id (default: active task)"`
	Status  string   `json:"status,omitempty" jsonschema:"todo, in_progress, blocked, or cancelled (done is set by close)"`
	ACDone  []int    `json:"ac_done,omitempty" jsonschema:"1-based numbers of criteria now satisfied"`
	AddAC   []string `json:"add_ac,omitempty" jsonschema:"criteria to add"`
	Note    string   `json:"note,omitempty" jsonschema:"note to append"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tags to add"`
	Profile string   `json:"profile,omitempty" jsonschema:"pin risk profile"`
}
type auditIn struct {
	Tasks bool `json:"tasks,omitempty" jsonschema:"create fix tasks from the findings"`
}

type taskNextIn struct {
	Goal  string `json:"goal,omitempty" jsonschema:"only tasks of this goal"`
	Tag   string `json:"tag,omitempty" jsonschema:"only tasks with this tag"`
	Start bool   `json:"start,omitempty" jsonschema:"start the first ready task"`
}

type taskListIn struct {
	Status string `json:"status,omitempty" jsonschema:"only this status"`
	All    bool   `json:"all,omitempty" jsonschema:"include done and cancelled"`
}
type closeIn struct {
	Summary   string   `json:"summary" jsonschema:"what changed, in one paragraph"`
	Knowledge string   `json:"knowledge" jsonschema:"one lasting finding, or the word none"`
	Next      []string `json:"next,omitempty" jsonschema:"next steps for whoever continues"`
	Tags      []string `json:"tags,omitempty" jsonschema:"tags for the knowledge entry"`
	Status    string   `json:"status,omitempty" jsonschema:"only to hand over unfinished work: in_progress or blocked"`
}
type reviewIn struct {
	Findings int `json:"findings" jsonschema:"problems found in this pass (0 if none)"`
}
type searchIn struct {
	Query string `json:"query" jsonschema:"what to look for"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 10)"`
}
type knowledgeIn struct {
	Text string   `json:"text" jsonschema:"the finding, one or two sentences"`
	Tags []string `json:"tags,omitempty" jsonschema:"tags; the first is the topic"`
	Pin  bool     `json:"pin,omitempty" jsonschema:"always show in briefs"`
}
type titleIn struct {
	Title string `json:"title" jsonschema:"decision title"`
}
type switchIn struct {
	Tool string `json:"tool" jsonschema:"target tool: claude-code, codex, gemini-cli, opencode, or another name"`
	Note string `json:"note,omitempty" jsonschema:"what the next tool should know"`
}
type reportIn struct {
	Since string `json:"since,omitempty" jsonschema:"period, e.g. 24h, 7d, or 2026-10-01 (default 7d)"`
}
type optIDIn struct {
	ID string `json:"id,omitempty" jsonschema:"task id (default: the active task)"`
}

func withID(a []string, id string) []string {
	if id != "" {
		return append(a, "--", id)
	}
	return a
}

type doctorIn struct {
	Fix bool `json:"fix,omitempty" jsonschema:"repair what is safe"`
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
