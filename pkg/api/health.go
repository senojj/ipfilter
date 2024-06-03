package api

import (
	"firehol/pkg/badip"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func Health(l *badip.List, r time.Duration) gin.HandlerFunc {
	nextRefresh := l.LastRefresh.Add(r)
	return func(c *gin.Context) {
		// capture the value of l.LastRefresh since we will be performing
		// multiple, separate calculations using the value. it is undesirable
		// for the value to be changed between calculations.
		lastRefresh := l.LastRefresh
		if lastRefresh.Equal(nextRefresh) || lastRefresh.After(nextRefresh) {
			nextRefresh = lastRefresh
			c.JSON(http.StatusOK, gin.H{})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{})
		}
	}
}
