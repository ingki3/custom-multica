package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// Workspace MCP Servers
// ---------------------------------------------------------------------------

type mcpServerResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   *string           `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       *string           `json:"url,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

func mcpServerToResponse(s db.WorkspaceMcpServer) mcpServerResponse {
	resp := mcpServerResponse{
		ID:        uuidToString(s.ID),
		Name:      s.Name,
		Transport: s.Transport,
		CreatedAt: s.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: s.UpdatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
	if s.Command.Valid {
		resp.Command = &s.Command.String
	}
	if s.Url.Valid {
		resp.URL = &s.Url.String
	}
	var args []string
	if len(s.Args) > 0 {
		json.Unmarshal(s.Args, &args)
	}
	resp.Args = args
	var env map[string]string
	if len(s.Env) > 0 {
		json.Unmarshal(s.Env, &env)
	}
	resp.Env = env
	return resp
}

func (h *Handler) ListMcpServers(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	servers, err := h.Queries.ListMcpServers(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list mcp servers")
		return
	}
	result := make([]mcpServerResponse, 0, len(servers))
	for _, s := range servers {
		result = append(result, mcpServerToResponse(s))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) CreateMcpServer(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())

	var req struct {
		Name      string            `json:"name"`
		Transport string            `json:"transport"`
		Command   *string           `json:"command"`
		Args      []string          `json:"args"`
		URL       *string           `json:"url"`
		Env       map[string]string `json:"env"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Transport == "" {
		req.Transport = "stdio"
	}

	argsJSON := []byte("[]")
	if len(req.Args) > 0 {
		argsJSON, _ = json.Marshal(req.Args)
	}
	envJSON := []byte("{}")
	if len(req.Env) > 0 {
		envJSON, _ = json.Marshal(req.Env)
	}

	params := db.CreateMcpServerParams{
		WorkspaceID: parseUUID(workspaceID),
		Name:        req.Name,
		Transport:   req.Transport,
		Args:        argsJSON,
		Env:         envJSON,
		CreatedBy:   parseUUID(userID),
	}
	if req.Command != nil {
		params.Command.String = *req.Command
		params.Command.Valid = true
	}
	if req.URL != nil {
		params.Url.String = *req.URL
		params.Url.Valid = true
	}

	server, err := h.Queries.CreateMcpServer(r.Context(), params)
	if err != nil {
		slog.Error("create mcp server failed", "error", err, "name", req.Name, "workspace_id", workspaceID, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "failed to create mcp server")
		return
	}

	h.publish(protocol.EventIssueUpdated, workspaceID, "member", userID, map[string]any{
		"mcp_server_created": true,
	})

	writeJSON(w, http.StatusCreated, mcpServerToResponse(server))
}

func (h *Handler) UpdateMcpServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	serverUUID, ok := parseUUIDOrBadRequest(w, serverID, "id")
	if !ok {
		return
	}

	var req struct {
		Name      *string           `json:"name"`
		Transport *string           `json:"transport"`
		Command   *string           `json:"command"`
		Args      []string          `json:"args"`
		URL       *string           `json:"url"`
		Env       map[string]string `json:"env"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := db.UpdateMcpServerParams{ID: serverUUID}
	if req.Name != nil {
		params.Name.String = *req.Name
		params.Name.Valid = true
	}
	if req.Transport != nil {
		params.Transport.String = *req.Transport
		params.Transport.Valid = true
	}
	if req.Command != nil {
		params.Command.String = *req.Command
		params.Command.Valid = true
	}
	if req.Args != nil {
		argsJSON, _ := json.Marshal(req.Args)
		params.Args = argsJSON
	}
	if req.URL != nil {
		params.Url.String = *req.URL
		params.Url.Valid = true
	}
	if req.Env != nil {
		envJSON, _ := json.Marshal(req.Env)
		params.Env = envJSON
	}

	server, err := h.Queries.UpdateMcpServer(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update mcp server")
		return
	}

	writeJSON(w, http.StatusOK, mcpServerToResponse(server))
}

func (h *Handler) DeleteMcpServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	serverUUID, ok := parseUUIDOrBadRequest(w, serverID, "id")
	if !ok {
		return
	}

	if err := h.Queries.DeleteMcpServer(r.Context(), serverUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete mcp server")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Agent MCP Server association
// ---------------------------------------------------------------------------

func (h *Handler) ListAgentMcpServers(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	agentUUID, ok := parseUUIDOrBadRequest(w, agentID, "agent id")
	if !ok {
		return
	}

	servers, err := h.Queries.ListAgentMcpServers(r.Context(), agentUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent mcp servers")
		return
	}

	type agentMcpResp struct {
		mcpServerResponse
		EnvOverride map[string]string `json:"env_override,omitempty"`
	}

	result := make([]agentMcpResp, 0, len(servers))
	for _, s := range servers {
		resp := agentMcpResp{
			mcpServerResponse: mcpServerResponse{
				ID:        uuidToString(s.ID),
				Name:      s.Name,
				Transport: s.Transport,
				CreatedAt: s.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt: s.UpdatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			},
		}
		if s.Command.Valid {
			resp.Command = &s.Command.String
		}
		if s.Url.Valid {
			resp.URL = &s.Url.String
		}
		var args []string
		if len(s.Args) > 0 {
			json.Unmarshal(s.Args, &args)
		}
		resp.Args = args
		var env map[string]string
		if len(s.Env) > 0 {
			json.Unmarshal(s.Env, &env)
		}
		resp.mcpServerResponse.Env = env
		if len(s.EnvOverride) > 0 {
			var override map[string]string
			json.Unmarshal(s.EnvOverride, &override)
			resp.EnvOverride = override
		}
		result = append(result, resp)
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) SetAgentMcpServers(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	agentUUID, ok := parseUUIDOrBadRequest(w, agentID, "agent id")
	if !ok {
		return
	}

	var req struct {
		Servers []struct {
			McpServerID string            `json:"mcp_server_id"`
			EnvOverride map[string]string `json:"env_override"`
		} `json:"servers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Clear existing and re-add
	if err := h.Queries.RemoveAllAgentMcpServers(r.Context(), agentUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear agent mcp servers")
		return
	}

	for _, s := range req.Servers {
		serverUUID, ok := parseUUIDOrBadRequest(w, s.McpServerID, "mcp_server_id")
		if !ok {
			return
		}
		envJSON, _ := json.Marshal(s.EnvOverride)
		if err := h.Queries.AddAgentMcpServer(r.Context(), db.AddAgentMcpServerParams{
			AgentID:     agentUUID,
			McpServerID: serverUUID,
			EnvOverride: envJSON,
		}); err != nil {
			slog.Warn("add agent mcp server failed", "agent_id", agentID, "mcp_server_id", s.McpServerID, "error", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
