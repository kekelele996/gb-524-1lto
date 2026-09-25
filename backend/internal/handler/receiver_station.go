package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/service"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type StationHandler struct {
	service *service.StationService
}

func NewStationHandler(stationService *service.StationService) *StationHandler {
	return &StationHandler{service: stationService}
}

func (h *StationHandler) List(c *gin.Context) {
	page, pageSize := pagination(c)
	stations, total, err := h.service.List(c.Request.Context(), page, pageSize, c.Query("status"))
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Page(c, stations, page, pageSize, total)
}

func (h *StationHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	station, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Success(c, http.StatusOK, station)
}

func (h *StationHandler) Create(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var request dto.CreateStationRequest
	if !bindJSON(c, &request) {
		return
	}
	station, err := h.service.Create(c.Request.Context(), request, actor)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Success(c, http.StatusCreated, station)
}

func (h *StationHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var request dto.UpdateStationRequest
	if !bindJSON(c, &request) {
		return
	}
	station, err := h.service.Update(c.Request.Context(), id, request, actor)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Success(c, http.StatusOK, station)
}

func (h *StationHandler) Coverage(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	coverage, err := h.service.Coverage(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Success(c, http.StatusOK, coverage)
}

// ---------------------------------------------------------------------------
// 测向站维护窗口 HTTP 适配
// ---------------------------------------------------------------------------

type MaintenanceHandler struct {
	service *service.MaintenanceService
}

func NewMaintenanceHandler(maintenanceService *service.MaintenanceService) *MaintenanceHandler {
	return &MaintenanceHandler{service: maintenanceService}
}

func (h *MaintenanceHandler) List(c *gin.Context) {
	page, pageSize := pagination(c)
	stationID, _ := strconv.ParseUint(c.Query("station_id"), 10, 32)
	windows, total, err := h.service.List(c.Request.Context(), uint(stationID), page, pageSize)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Page(c, windows, page, pageSize, total)
}

func (h *MaintenanceHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	window, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Success(c, http.StatusOK, window)
}

func (h *MaintenanceHandler) Register(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var request dto.CreateMaintenanceWindowRequest
	if !bindJSON(c, &request) {
		return
	}
	window, err := h.service.Register(c.Request.Context(), request, actor)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Success(c, http.StatusCreated, window)
}

func (h *MaintenanceHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var request dto.UpdateMaintenanceWindowRequest
	if !bindJSON(c, &request) {
		return
	}
	window, err := h.service.Update(c.Request.Context(), id, request, actor)
	if err != nil {
		api.Fail(c, err)
		return
	}
	api.Success(c, http.StatusOK, window)
}
