package api

import (
	"context"
	"net/http"

	"github.com/oapi-codegen/nullable"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/httpx"
)

// GetMqttStatus returns the state of the connection, the target lists
// offered by Home Assistant and the chosen one (architecture.md, 11.7).
// Without MQTT the state is disabled and nothing is offered.
func (s *Server) GetMqttStatus(ctx context.Context, request GetMqttStatusRequestObject) (GetMqttStatusResponseObject, error) {
	status, err := s.mqttStatus(ctx)
	if err != nil {
		return nil, err
	}
	return GetMqttStatus200JSONResponse(status), nil
}

// SetShoppingTarget chooses the offered target list with the id of the
// request, or removes the choice for id null, and returns the status
// afterwards (architecture.md, 11.7). Without MQTT it returns 409
// mqtt_disabled, for an id that is not offered 422 unknown_target.
func (s *Server) SetShoppingTarget(ctx context.Context, request SetShoppingTargetRequestObject) (SetShoppingTargetResponseObject, error) {
	if s.deps.MQTT == nil {
		return nil, httpx.NewError(http.StatusConflict, "mqtt_disabled", "MQTT ist nicht eingerichtet")
	}
	var target *domain.ShoppingTarget
	if !request.Body.Id.IsNull() {
		id, err := request.Body.Id.Get()
		if err != nil {
			return nil, httpx.BadRequest("id fehlt")
		}
		t, err := domain.FindShoppingTarget(s.deps.MQTT.Offer(), id)
		if err != nil {
			return nil, err
		}
		target = &t
	}
	if err := s.deps.MQTT.SetTarget(ctx, target); err != nil {
		return nil, err
	}
	status, err := s.mqttStatus(ctx)
	if err != nil {
		return nil, err
	}
	return SetShoppingTarget200JSONResponse(status), nil
}

// mqttStatus returns the MqttStatus with the stored choice, also if it is
// not offered right now.
func (s *Server) mqttStatus(ctx context.Context) (MqttStatus, error) {
	status := MqttStatus{Status: Disabled, Targets: []ShoppingTarget{}}
	if s.deps.MQTT != nil {
		status.Status = MqttStatusStatus(s.deps.MQTT.State())
		for _, t := range s.deps.MQTT.Offer() {
			status.Targets = append(status.Targets, ShoppingTarget{Id: t.ID, Name: t.Name})
		}
	}
	stored, err := domain.StoredShoppingTarget(ctx, s.queries)
	if err != nil {
		return MqttStatus{}, err
	}
	if stored == nil {
		status.Target = nullable.NewNullNullable[ShoppingTarget]()
	} else {
		status.Target = nullable.NewNullableWithValue(ShoppingTarget{Id: stored.ID, Name: stored.Name})
	}
	return status, nil
}
