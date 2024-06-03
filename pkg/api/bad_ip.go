package api

import (
	"firehol/pkg/badip"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"strings"
)

type msg struct {
	Message string `json:"message"`
}

type result struct {
	IsBadIP bool `json:"is_bad_ip"`
}

func IsBadIP(c *gin.Context) {
	list := c.MustGet("bad_ip_list").(*badip.List)
	address := c.Query("address")
	if strings.TrimSpace(address) == "" {
		c.JSON(http.StatusBadRequest, msg{
			Message: "missing address parameter value",
		})
		return
	}
	ip := net.ParseIP(address)
	if ip == nil {
		c.JSON(http.StatusBadRequest, msg{
			Message: "invalid IP address format",
		})
		return
	}
	c.JSON(http.StatusOK, result{
		IsBadIP: list.Contains(ip),
	})
}
