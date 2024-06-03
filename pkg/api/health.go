package api

import (
	"github.com/gin-gonic/gin"
	"ipfilter/pkg/iplist"
	"net/http"
	"time"
)

// Health creates an endpoint handler that indicates the "freshness" of the
// iplist.List data. If the last refresh of the iplist.List data has become
// stale, based on the expected refresh duration, the service is considered
// to be unhealthy.
func Health(l *iplist.List, r time.Duration) gin.HandlerFunc {
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
