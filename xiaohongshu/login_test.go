package xiaohongshu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyLoginState(t *testing.T) {
	tests := []struct {
		name    string
		signals loginPageSignals
		want    LoginState
	}{
		{
			name: "device verification is not final login",
			signals: loginPageSignals{
				SiteMatched:           true,
				Authenticated:         true,
				DeviceVerificationQR:  "device-qr",
				InitialLoginQRCode:    "login-qr",
				LoginContainerVisible: true,
			},
			want: LoginState{
				Stage:  LoginStageDeviceVerification,
				QRCode: "device-qr",
			},
		},
		{
			name: "authenticated page without verification is logged in",
			signals: loginPageSignals{
				SiteMatched:           true,
				Authenticated:         true,
				InitialLoginQRCode:    "login-qr",
				LoginContainerVisible: true,
			},
			want: LoginState{Stage: LoginStageLoggedIn},
		},
		{
			name: "device verification QR takes precedence",
			signals: loginPageSignals{
				SiteMatched:           true,
				DeviceVerificationQR:  "device-qr",
				InitialLoginQRCode:    "login-qr",
				LoginContainerVisible: true,
			},
			want: LoginState{
				Stage:  LoginStageDeviceVerification,
				QRCode: "device-qr",
			},
		},
		{
			name: "initial login QR",
			signals: loginPageSignals{
				SiteMatched:           true,
				InitialLoginQRCode:    "login-qr",
				LoginContainerVisible: true,
			},
			want: LoginState{
				Stage:  LoginStageQRCode,
				QRCode: "login-qr",
			},
		},
		{
			name: "waiting for mobile confirmation",
			signals: loginPageSignals{
				SiteMatched:           true,
				LoginContainerVisible: true,
			},
			want: LoginState{Stage: LoginStageWaitingConfirmation},
		},
		{
			name: "authentication signal from another domain is rejected",
			signals: loginPageSignals{
				Authenticated: true,
			},
			want: LoginState{Stage: LoginStageUnknown},
		},
		{
			name: "unknown state",
			signals: loginPageSignals{
				SiteMatched: true,
			},
			want: LoginState{Stage: LoginStageUnknown},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyLoginState(tt.signals))
		})
	}
}
