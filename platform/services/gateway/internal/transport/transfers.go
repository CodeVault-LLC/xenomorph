package transport

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
	"github.com/gin-gonic/gin"
)

type agentTransferProgressResponse struct {
	AcknowledgedChunks []int `json:"acknowledged_chunks"`
	BytesVerified      int64 `json:"bytes_verified"`
}
type agentTransferFinalizeResponse struct {
	State         fileworkspace.TransferState `json:"state"`
	BytesVerified int64                       `json:"bytes_verified"`
}

func (s *Server) handleAgentTransferChunkPut(c *gin.Context) {
	agentID, token, index, ok := s.agentTransferScope(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFileChunkBytes)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transfer chunk"})
		return
	}
	transfer, err := s.fileWorkspace.PutAgentTransferChunk(agentID, c.Param("transferID"), token, index, data)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "transfer chunk rejected"})
		return
	}
	c.JSON(http.StatusOK, agentTransferProgressResponse{AcknowledgedChunks: transfer.Acknowledged, BytesVerified: transfer.BytesVerified})
}

func (s *Server) handleAgentTransferChunkGet(c *gin.Context) {
	agentID, token, index, ok := s.agentTransferScope(c)
	if !ok {
		return
	}
	data, err := s.fileWorkspace.ReadAgentTransferChunk(agentID, c.Param("transferID"), token, index)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "transfer chunk unavailable"})
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/octet-stream", data)
}

func (s *Server) handleAgentTransferFinalize(c *gin.Context) {
	if s.fileWorkspace == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "file workspace unavailable"})
		return
	}
	agentID := c.GetString("agent_id")
	token, ok := bearerToken(c.GetHeader("Authorization"))
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "transfer capability required"})
		return
	}
	transfer, err := s.fileWorkspace.FinalizeAgentTransfer(agentID, c.Param("transferID"), token)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "transfer finalization failed"})
		return
	}
	c.JSON(http.StatusOK, agentTransferFinalizeResponse{State: transfer.State, BytesVerified: transfer.BytesVerified})
}

func (s *Server) agentTransferScope(c *gin.Context) (string, string, int, bool) {
	if s.fileWorkspace == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "file workspace unavailable"})
		return "", "", 0, false
	}
	token, ok := bearerToken(c.GetHeader("Authorization"))
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "transfer capability required"})
		return "", "", 0, false
	}
	index, err := strconv.Atoi(c.Param("chunkIndex"))
	if err != nil || index < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transfer chunk index"})
		return "", "", 0, false
	}
	return c.GetString("agent_id"), token, index, true
}

func bearerToken(value string) (string, bool) {
	prefix, token, found := strings.Cut(strings.TrimSpace(value), " ")
	return token, found && prefix == "Bearer" && len(token) == 64
}
