package controllers

import (
	"net/http"
	"phynixdrive/services"
	"phynixdrive/utils"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type SearchController struct {
	searchService *services.SearchService
}

func NewSearchController(db *mongo.Database, permService *services.PermissionService) *SearchController {
	return &SearchController{
		searchService: services.NewSearchService(db, permService),
	}
}

// Search performs a general search across files and folders
func (sc *SearchController) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Search query required", nil)
		return
	}

	userId := c.GetString("userId")
	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional parameters
	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 50
	}

	offsetInt, err := strconv.Atoi(offset)
	if err != nil || offsetInt < 0 {
		offsetInt = 0
	}

	results, err := sc.searchService.Search(userId, query, limitInt, offsetInt)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Search failed", nil)
		return
	}

	utils.SuccessResponse(c, "Search completed", results)
}

// SearchFilesOnly searches only files
func (sc *SearchController) SearchFilesOnly(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Search query required", nil)
		return
	}

	userId := c.GetString("userId")
	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional parameters
	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 50
	}

	offsetInt, err := strconv.Atoi(offset)
	if err != nil || offsetInt < 0 {
		offsetInt = 0
	}

	files, err := sc.searchService.SearchFilesOnly(userId, query, limitInt, offsetInt)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "File search failed", nil)
		return
	}

	utils.SuccessResponse(c, "Files search completed", files)
}

// SearchFoldersOnly searches only folders
func (sc *SearchController) SearchFoldersOnly(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Search query required", nil)
		return
	}

	userId := c.GetString("userId")
	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional parameters
	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 50
	}

	offsetInt, err := strconv.Atoi(offset)
	if err != nil || offsetInt < 0 {
		offsetInt = 0
	}

	folders, err := sc.searchService.SearchFoldersOnly(userId, query, limitInt, offsetInt)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Folder search failed", nil)
		return
	}

	utils.SuccessResponse(c, "Folders search completed", folders)
}

// GetRecentFiles retrieves recently accessed/modified files
func (sc *SearchController) GetRecentFiles(c *gin.Context) {
	userId := c.GetString("userId")
	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional parameters
	limit := c.DefaultQuery("limit", "20")
	days := c.DefaultQuery("days", "30") // Recent files from last 30 days

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 20
	}

	daysInt, err := strconv.Atoi(days)
	if err != nil || daysInt <= 0 {
		daysInt = 30
	}

	files, err := sc.searchService.GetRecentFiles(userId, limitInt, daysInt)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve recent files", nil)
		return
	}
	utils.SuccessResponse(c, "Recent files retrieved", files)
}

// GetSharedWithMe retrieves files and folders shared with the current user
func (sc *SearchController) GetSharedWithMe(c *gin.Context) {
	userId := c.GetString("userId")
	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional parameters
	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")
	itemType := c.DefaultQuery("type", "all") // "files", "folders", or "all"

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 50
	}

	offsetInt, err := strconv.Atoi(offset)
	if err != nil || offsetInt < 0 {
		offsetInt = 0
	}

	// Validate item type
	if itemType != "files" && itemType != "folders" && itemType != "all" {
		itemType = "all"
	}

	sharedItems, err := sc.searchService.GetSharedWithMe(userId, itemType, limitInt, offsetInt)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get shared items", nil)
		return
	}

	utils.SuccessResponse(c, "Shared items retrieved", sharedItems)
}
