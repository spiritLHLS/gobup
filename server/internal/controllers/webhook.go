package controllers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/webhook"
)

func RecordWebHook(c *gin.Context) {
	var event interface{}
	if err := c.ShouldBindJSON(&event); err != nil {
		log.Printf("[ERROR] Failed to parse webhook event: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Received webhook event: %+v", event)

	// 检查是否为同步处理模式（通过查询参数 sync=true）
	syncMode := c.Query("sync") == "true"

	processor := webhook.NewProcessor()

	if syncMode {
		// 同步处理，立即返回处理结果
		log.Printf("[DEBUG] 同步处理模式")
		if err := processor.Process(event); err != nil {
			log.Printf("[ERROR] Failed to process event: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   err.Error(),
				"status":  "error",
				"message": "处理webhook事件失败",
			})
			return
		}
		log.Printf("[INFO] 同步处理完成")
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "处理成功"})
	} else {
		// 异步处理（原有逻辑）
		log.Printf("[DEBUG] 异步处理模式")
		go func() {
			if err := processor.Process(event); err != nil {
				log.Printf("[ERROR] Failed to process event: %v", err)
			} else {
				log.Printf("[INFO] 异步处理完成")
			}
		}()
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "已接受，正在处理"})
	}
}

func RecordWebHookGet(c *gin.Context) {
	c.String(http.StatusOK, "Webhook endpoint is active")
}
