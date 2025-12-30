package controllers

import (
"net/http"

"github.com/gin-gonic/gin"
"github.com/gobup/server/internal/database"
"github.com/gobup/server/internal/models"
)

func ListParts(c *gin.Context) {
	historyID := c.Param("id")
	
	db := database.GetDB()
	var parts []models.RecordHistoryPart
	db.Where("history_id = ?", historyID).Order("start_time ASC").Find(&parts)
	
	c.JSON(http.StatusOK, parts)
}

func UploadToEditor(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"type": "info", "msg": "功能开发中"})
}
