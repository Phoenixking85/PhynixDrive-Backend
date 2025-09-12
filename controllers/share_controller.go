package controllers

import (
	"net/http"
	"phynixdrive/services"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type ShareController struct {
	shareService *services.ShareService
	validator    *validator.Validate
}

type BulkShareRequest struct {
	Resources []struct {
		ResourceID   string `json:"resource_id" validate:"required"`
		ResourceType string `json:"resource_type" validate:"required,oneof=file folder"`
	} `json:"resources" validate:"required,min=1,max=50"`
	Email             string `json:"email" validate:"required,email"`
	Role              string `json:"role" validate:"required,oneof=viewer editor admin"`
	InheritToChildren bool   `json:"inherit_to_children,omitempty"`
}

type BulkShareResponse struct {
	Successful []services.ShareResponse `json:"successful"`
	Failed     []BulkShareError         `json:"failed"`
	Summary    BulkShareSummary         `json:"summary"`
}

type BulkShareError struct {
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	Error        string `json:"error"`
}

type BulkShareSummary struct {
	Total      int `json:"total"`
	Successful int `json:"successful"`
	Failed     int `json:"failed"`
}

type UpdatePermissionRequest struct {
	Role string `json:"role" validate:"required,oneof=viewer editor admin"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func NewShareController(shareService *services.ShareService) *ShareController {
	return &ShareController{
		shareService: shareService,
		validator:    validator.New(),
	}
}

// ShareResource
func (sc *ShareController) ShareResource(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	var request services.ShareRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	if err := sc.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_failed",
			Message: err.Error(),
		})
		return
	}

	// Normalize email to lowercase
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))

	response, err := sc.shareService.ShareResource(c.Request.Context(), request, userID.(string))
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			statusCode = http.StatusNotFound
		} else if strings.Contains(err.Error(), "insufficient permissions") {
			statusCode = http.StatusForbidden
		} else if strings.Contains(err.Error(), "already shared") {
			statusCode = http.StatusConflict
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   "share_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, SuccessResponse{
		Message: "Resource shared successfully",
		Data:    response,
	})
}

// BulkShare handles
func (sc *ShareController) BulkShare(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	var request BulkShareRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	if err := sc.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_failed",
			Message: err.Error(),
		})
		return
	}

	// Normalize email
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))

	var successful []services.ShareResponse
	var failed []BulkShareError

	for _, resource := range request.Resources {
		shareRequest := services.ShareRequest{
			ResourceID:        resource.ResourceID,
			ResourceType:      resource.ResourceType,
			Email:             request.Email,
			Role:              request.Role,
			InheritToChildren: request.InheritToChildren,
		}

		response, err := sc.shareService.ShareResource(c.Request.Context(), shareRequest, userID.(string))
		if err != nil {
			failed = append(failed, BulkShareError{
				ResourceID:   resource.ResourceID,
				ResourceType: resource.ResourceType,
				Error:        err.Error(),
			})
		} else {
			successful = append(successful, *response)
		}
	}

	bulkResponse := BulkShareResponse{
		Successful: successful,
		Failed:     failed,
		Summary: BulkShareSummary{
			Total:      len(request.Resources),
			Successful: len(successful),
			Failed:     len(failed),
		},
	}

	statusCode := http.StatusOK
	if len(successful) > 0 && len(failed) == 0 {
		statusCode = http.StatusCreated
	} else if len(successful) == 0 {
		statusCode = http.StatusBadRequest
	}

	c.JSON(statusCode, bulkResponse)
}

// GetSharedByMe
func (sc *ShareController) GetSharedByMe(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	// Get optional resource type filter
	resourceType := c.Query("type")
	var resourceTypePtr *string
	if resourceType != "" && (resourceType == "file" || resourceType == "folder") {
		resourceTypePtr = &resourceType
	}

	shares, err := sc.shareService.GetSharedByMe(c.Request.Context(), userID.(string), resourceTypePtr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Resources shared by you retrieved successfully",
		Data: gin.H{
			"shares": shares,
			"total":  len(shares),
		},
	})
}

// GetSharedWithMe
func (sc *ShareController) GetSharedWithMe(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	// Get optional resource type filter
	resourceType := c.Query("type")
	var resourceTypePtr *string
	if resourceType != "" && (resourceType == "file" || resourceType == "folder") {
		resourceTypePtr = &resourceType
	}

	resources, err := sc.shareService.GetSharedWithMe(c.Request.Context(), userID.(string), resourceTypePtr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Resources shared with you retrieved successfully",
		Data: gin.H{
			"resources": resources,
			"total":     len(resources),
		},
	})
}

// GetAllSharedResources
func (sc *ShareController) GetAllSharedResources(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	response, err := sc.shareService.GetAllSharedResources(c.Request.Context(), userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "All shared resources retrieved successfully",
		Data:    response,
	})
}

// GetResourcePermissions
func (sc *ShareController) GetResourcePermissions(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	resourceType := c.Param("resource_type")
	resourceID := c.Param("resource_id")

	if resourceType != "file" && resourceType != "folder" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_resource_type",
			Message: "Resource type must be 'file' or 'folder'",
		})
		return
	}

	if resourceID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_resource_id",
			Message: "Resource ID is required",
		})
		return
	}

	permissions, err := sc.shareService.GetResourcePermissions(c.Request.Context(), resourceID, resourceType, userID.(string))
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "insufficient permissions") {
			statusCode = http.StatusForbidden
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   "fetch_permissions_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Resource permissions retrieved successfully",
		Data: gin.H{
			"permissions": permissions,
			"total":       len(permissions),
		},
	})
}

// RevokePermission
func (sc *ShareController) RevokePermission(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	shareID := c.Param("share_id")
	if shareID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_share_id",
			Message: "Share ID is required",
		})
		return
	}

	err := sc.shareService.RevokePermission(c.Request.Context(), shareID, userID.(string))
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			statusCode = http.StatusNotFound
		} else if strings.Contains(err.Error(), "insufficient permissions") {
			statusCode = http.StatusForbidden
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   "revoke_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Permission revoked successfully",
	})
}

// UpdatePermission
func (sc *ShareController) UpdatePermission(c *gin.Context) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	shareID := c.Param("share_id")
	if shareID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_share_id",
			Message: "Share ID is required",
		})
		return
	}

	var request UpdatePermissionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	if err := sc.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_failed",
			Message: err.Error(),
		})
		return
	}

	response, err := sc.shareService.UpdatePermission(c.Request.Context(), shareID, request.Role, userID.(string))
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			statusCode = http.StatusNotFound
		} else if strings.Contains(err.Error(), "insufficient permissions") {
			statusCode = http.StatusForbidden
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   "update_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Permission updated successfully",
		Data:    response,
	})
}

// GetShareDetails handles GET /api/share/:share_id
func (sc *ShareController) GetShareDetails(c *gin.Context) {
	_, exists := c.Get("userIdStr")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
		})
		return
	}

	shareID := c.Param("share_id")
	if shareID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_share_id",
			Message: "Share ID is required",
		})
		return
	}

	c.JSON(http.StatusNotImplemented, ErrorResponse{
		Error:   "not_implemented",
		Message: "GetShareDetails method needs to be implemented in ShareService",
	})
}
