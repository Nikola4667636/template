package trip

import "github.com/google/uuid"

const nonEmptyUUIDReason = "must be a non-empty UUID"

type CreateTripParams struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
}

func (p CreateTripParams) Validate() error {
	if p.UserID == uuid.Nil {
		return &ValidationError{Field: "user_id", Reason: nonEmptyUUIDReason}
	}
	if p.DriverID == uuid.Nil {
		return &ValidationError{Field: "driver_id", Reason: nonEmptyUUIDReason}
	}
	if p.Price < 0 {
		return &ValidationError{Field: "price", Reason: "must not be negative"}
	}
	if err := validateCoordinates("start_point", p.StartPoint); err != nil {
		return err
	}
	return validateCoordinates("end_point", p.EndPoint)
}

func validateCoordinates(field string, point Coordinates) error {
	// Inclusive comparisons also reject NaN, which does not belong to either range.
	if !(point.Latitude >= -90 && point.Latitude <= 90) {
		return &ValidationError{
			Field:  field + ".latitude",
			Reason: "must be between -90 and 90",
		}
	}
	if !(point.Longitude >= -180 && point.Longitude <= 180) {
		return &ValidationError{
			Field:  field + ".longitude",
			Reason: "must be between -180 and 180",
		}
	}
	return nil
}
