package handlers

import (
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"
	"crypto/hmac"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type cronTasksReader interface {
	List() ([]models.CronTask, error)
}

type AdminCronTaskHandler struct {
	store     cronTasksReader
	scheduler *services.CronTaskScheduler
}

func NewAdminCronTaskHandler(store cronTasksReader, scheduler *services.CronTaskScheduler) *AdminCronTaskHandler {
	return &AdminCronTaskHandler{store: store, scheduler: scheduler}
}

func (h *AdminCronTaskHandler) render(c *gin.Context, status int, messageKey, message string) {
	tasks, err := h.store.List()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "加载任务失败"})
		return
	}
	common := getCommonAdminData(c, "cron-tasks", "定时任务", "管理内置任务的每日执行时间及状态。所有时间使用 UTC。")
	common["tasks"] = tasks
	common["csrfToken"] = settingsCSRF(c)
	nextRuns := make(map[string]string, len(tasks))
	for _, task := range tasks {
		if task.Enabled {
			next, err := h.scheduler.NextRun(task, time.Now().UTC())
			if err == nil {
				nextRuns[task.Key] = next.Format("2006-01-02 15:04 UTC")
			}
		}
	}
	common["nextRuns"] = nextRuns
	if messageKey != "" {
		common[messageKey] = message
	}
	c.HTML(status, "admin-cron-tasks.html", common)
}

func (h *AdminCronTaskHandler) List(c *gin.Context) {
	if c.Query("run") == "started" {
		h.render(c, http.StatusOK, "success", "手动任务已启动，请稍后刷新查看运行结果")
		return
	}
	h.render(c, http.StatusOK, "", "")
}

func (h *AdminCronTaskHandler) RunManual(c *gin.Context) {
	expected := settingsCSRF(c)
	if expected == "" || !hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token"))) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if err := h.scheduler.StartManual(c.Param("key")); err != nil {
		if errors.Is(err, services.ErrTaskRunning) {
			c.String(http.StatusConflict, "任务正在运行，请稍后重试")
		} else {
			c.String(http.StatusBadRequest, "手动执行失败：%s", err.Error())
		}
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/cron-tasks?run=started")
}

func (h *AdminCronTaskHandler) Update(c *gin.Context) {
	expected := settingsCSRF(c)
	if expected == "" || !hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token"))) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	spec := strings.TrimSpace(c.PostForm("cron_spec"))
	if err := h.scheduler.Update(c.Param("key"), spec, c.PostForm("enabled") == "1"); err != nil {
		c.String(http.StatusBadRequest, "定时任务配置无效：%s", err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/cron-tasks")
}
