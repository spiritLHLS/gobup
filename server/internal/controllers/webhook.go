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
		log.Printf("Failed to parse webhook event: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Received webhook event: %+v", event)

	go func() {
		processor := webhook.NewProcessor()
		if err := processor.Process(event); err != nil {
			log.Printf("Failed to process event: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func RecordWebHookGet(c *gin.Context) {
	c.String(http.StatusOK, "Webhook endpoint is active")
}
