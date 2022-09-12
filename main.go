package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/jcuga/golongpoll"
	"github.com/jcuga/golongpoll/client"
)

var	counter int64 = 0
 
func main() {
	manager, _ := golongpoll.StartLongpoll(golongpoll.Options{
		LoggingEnabled: true,
	})	
	
	manager2, _ := golongpoll.StartLongpoll(golongpoll.Options{
		LoggingEnabled: true,
	})

	router := gin.Default()
	router.Use(cors.Default())

	router.Static("/js", "./web/assets/js")
	router.HTMLRender = loadTemplates("./web/templates")

	router.GET("/api/vote/events", wrapWithContext(manager.SubscriptionHandler))
	router.POST("/api/vote/send", wrapWithContext(manager.PublishHandler))

	router.GET("/api/chat/events", wrapWithContext(manager2.SubscriptionHandler))
	router.POST("/api/chat/send", wrapWithContext(manager2.PublishHandler))

	router.GET("/api/chat", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H {
			"chats": chats,
		})
	})

	router.GET("/api/vote", func(ctx *gin.Context) {
		action := ctx.DefaultQuery("action", "data")
		if action == "data" {
			ctx.JSON(http.StatusOK, gin.H{
				"count": atomic.LoadInt64(&counter),
			})
			return
		} 
		// else if action == "add" {
		// 	atomic.AddInt64(&counter, 1)
		// 	ctx.JSON(http.StatusOK, gin.H{
		// 		"count": atomic.LoadInt64(&counter),
		// 	})
		// 	return
		// } else if action == "sub" {
		// 	if atomic.LoadInt64(&counter) > 0 {
		// 		atomic.AddInt64(&counter, -1)
		// 		ctx.JSON(http.StatusOK, gin.H{
		// 			"count": atomic.LoadInt64(&counter),
		// 		})
		// 		return
		// 	}
		// 	ctx.JSON(http.StatusOK, gin.H{
		// 		"count": 0,
		// 	})		
		// 	return	
		// } 
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Unknown action",
		})
	})

	router.GET("/", Index)
	go upPollCounter(manager)
	go upPollChat(manager2)

	router.Run(":3030")
}

func wrapWithContext(lpHandler func(http.ResponseWriter, *http.Request)) func(*gin.Context) {
	return func(c *gin.Context) {
		log.Println("before queries param:", c.Request.URL.Query())
		q := c.Request.URL.Query()
		q.Set("timeout", "5")
		c.Request.URL.RawQuery = q.Encode()
		log.Println("after queries param:", c.Request.URL.Query())
		lpHandler(c.Writer, c.Request)
	}
}


var chats = []map[string]string {
	{
		"from": "sammi",
		"message": "hello",
	},
} 

func upPollChat(ipManager *golongpoll.LongpollManager) {
	u, err := url.Parse("http://127.0.0.1:3030/api/chat/events")
	if err != nil {
		panic(err)
	}

	c, err := client.NewClient(client.ClientOptions{
		SubscribeUrl:   *u,
		Category:       "to-chat",
		LoggingEnabled: true,
	})
	if err != nil {
		fmt.Println("failed to create long polling: ", err)
		return
	}

	for e := range c.Start(time.Now()) {
		data := strings.Split(e.Data.(string), "=")
		
		from := data[0]
		msg := data[1]

		log.Println("from: ", from)
		log.Println("msg: ", msg)
		chats = append(chats, map[string]string{
			from: msg,
		})

		log.Println("data: ", chats)
		ipManager.Publish("from-chat", data)
	}
}

func upPollCounter(ipManager *golongpoll.LongpollManager) {
	u, err := url.Parse("http://127.0.0.1:3030/api/vote/events")
	if err != nil {
		panic(err)
	}

	c, err := client.NewClient(client.ClientOptions{
		SubscribeUrl:   *u,
		Category:       "to-counter",
		LoggingEnabled: true,
	})

	if err != nil {
		fmt.Println("failed to create long polling: ", err)
		return
	}

	for e := range c.Start(time.Now()) {
		action, _ := e.Data.(string) // add or sub
		if action == "add" {
			atomic.AddInt64(&counter, 1)
			ipManager.Publish("from-counter", atomic.LoadInt64(&counter))
		} else if action == "sub" {
			if atomic.LoadInt64(&counter) > 0 {
				atomic.AddInt64(&counter, -1)
			}
			ipManager.Publish("from-counter", atomic.LoadInt64(&counter))
		}
	}	
}

func Index(c *gin.Context)  {
	c.HTML(http.StatusOK, "index.tmpl", gin.H{
		"title": "Main website",
		"count": atomic.LoadInt64(&counter),
	})
}

func loadTemplates(templatesDir string) multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	layouts, err := filepath.Glob(templatesDir + "/layouts/*")
	if err != nil {
		panic(err.Error())
	}

	includes, err := filepath.Glob(templatesDir + "/**/*")
	if err != nil {
		panic(err.Error())
	}

	for _, include := range includes {
		layoutCopy := make([]string, len(layouts))
		copy(layoutCopy, layouts)
		files := append(layoutCopy, include)
		r.AddFromFiles(filepath.Base(include), files...)
	}
	return r
}
