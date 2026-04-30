//go:build !darwin

package sandbox

// SeatbeltProvider is a stub on non-darwin platforms.
type SeatbeltProvider struct{}

// NewSeatbeltProvider returns a stub SeatbeltProvider on non-darwin platforms.
func NewSeatbeltProvider() *SeatbeltProvider {
	return &SeatbeltProvider{}
}

func (p *SeatbeltProvider) Name() string    { return "seatbelt" }
func (p *SeatbeltProvider) Kind() string    { return "local" }
func (p *SeatbeltProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{Available: false, Platform: "darwin", ErrorMessage: "seatbelt not available on " + string(DetectOS())}, nil
}
func (p *SeatbeltProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, ErrProviderUnavailable
}

// 编译期断言：SeatbeltProvider 满足 Provider 接口
var _ Provider = (*SeatbeltProvider)(nil)
