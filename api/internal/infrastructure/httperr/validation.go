// api/internal/infrastructure/httperr/validation.go
package httperr

import "github.com/go-playground/validator/v10"

// validationToHTTPError maps a validator/v10 first-error to the matching
// captured code. Per spec §7.3.1, the current API only validates:
//
//	bad_visibility      (oneof on Visibility field)
//	bad_cover_position  (gte=0 on CoverPosition field)
//
// Everything else collapses to bad_json.
func validationToHTTPError(verrs validator.ValidationErrors) HTTPError {
	if len(verrs) == 0 {
		return HTTPError{Error: "bad_json"}
	}
	fe := verrs[0] // current API surfaces only the first failure
	switch {
	case fe.Field() == "Visibility" && fe.Tag() == "oneof":
		return HTTPError{Error: "bad_visibility"}
	case fe.Field() == "CoverPosition" && fe.Tag() == "gte":
		return HTTPError{Error: "bad_cover_position"}
	default:
		return HTTPError{Error: "bad_json"}
	}
}
