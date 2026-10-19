package domain

type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string     { return e.Message }
func (e *RequestError) PublicStatus() int { return e.Status }

var ErrForbidden = &RequestError{Status: 403, Message: "没有维护此资源的权限"}
