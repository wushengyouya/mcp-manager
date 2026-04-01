package dto

// IDPathRequest 定义通用路径参数
type IDPathRequest struct {
	ID string `uri:"id" binding:"required"`
}

// PageQuery 定义通用分页参数
type PageQuery struct {
	Page     *int `form:"page" binding:"omitempty,gt=0"`
	PageSize *int `form:"page_size" binding:"omitempty,gt=0,lte=100"`
}

// GetPage 返回规范化后的页码
func (q PageQuery) GetPage() int {
	if q.Page == nil {
		return 1
	}
	return *q.Page
}

// GetPageSize 返回规范化后的每页条数
func (q PageQuery) GetPageSize() int {
	if q.PageSize == nil {
		return 10
	}
	return *q.PageSize
}
