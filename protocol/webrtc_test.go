package protocol

import (
	"encoding/json"
	"testing"
)

var msgs = [...]string{
	`{"type":"ANSWER","src":"dest-peer-id","dst":"someid","payload":{"sdp":{"type":"answer","sdp":"v=0\r\no=- 8276888538055714041 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE data\r\na=msid-semantic: WMS\r\nm=application 9 DTLS/SCTP 5000\r\nc=IN IP4 0.0.0.0\r\nb=AS:30\r\na=ice-ufrag:g2yO\r\na=ice-pwd:7A7+wwgodBorD3KsLRQf2oNB\r\na=ice-options:trickle\r\na=fingerprint:sha-256 6D:3C:C9:74:8C:41:AC:3F:93:05:C0:98:44:26:D2:F6:15:95:F8:AC:63:14:22:FA:B7:9E:EC:10:1A:BC:76:7E\r\na=setup:active\r\na=mid:data\r\na=sctpmap:5000 webrtc-datachannel 1024\r\n"},"type":"data","connectionId":"dc_d29iaxm9wi","browser":"Chrome"}}`,
	`{"type":"OFFER","payload":{"sdp":{"type":"offer","sdp":"v=0\r\no=- 596709752457229267 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE data\r\na=msid-semantic: WMS\r\nm=application 9 DTLS/SCTP 5000\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:0pH9\r\na=ice-pwd:KMJGrCkj0RhH+hFZcwrmIR0l\r\na=ice-options:trickle\r\na=fingerprint:sha-256 0E:62:E9:D9:F6:48:05:5D:F4:EC:0A:8E:2C:48:0E:C3:5E:B2:9D:47:4E:F6:9D:6B:D8:B3:E1:30:58:39:35:41\r\na=setup:actpass\r\na=mid:data\r\na=sctpmap:5000 webrtc-datachannel 1024\r\n"},"type":"data","label":"dc_d29iaxm9wi","connectionId":"dc_d29iaxm9wi","reliable":false,"serialization":"binary","browser":"Chrome"},"dst":"dest-peer-id"}`,
	`{"type":"CANDIDATE","payload":{"candidate":{"candidate":"candidate:842163049 1 udp 1677729535 195.174.144.24 56801 typ srflx raddr 0.0.0.0 rport 0 generation 0 ufrag 0pH9 network-cost 50","sdpMid":"data","sdpMLineIndex":0},"type":"data","connectionId":"dc_d29iaxm9wi"},"dst":"dest-peer-id"}`,
	`{"type":"ID-TAKEN","payload":{"msg":"ID is taken"}}`,
}

var payloads = [...]string{
	`{"type":"data","connectionId":"dc_d29iaxm9wi","browser":"Chrome"}`,
	`{"sdp":{"type":"offer","sdp":"v=0\r\no=- 596709752457229267 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE data\r\na=msid-semantic: WMS\r\nm=application 9 DTLS/SCTP 5000\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:0pH9\r\na=ice-pwd:KMJGrCkj0RhH+hFZcwrmIR0l\r\na=ice-options:trickle\r\na=fingerprint:sha-256 0E:62:E9:D9:F6:48:05:5D:F4:EC:0A:8E:2C:48:0E:C3:5E:B2:9D:47:4E:F6:9D:6B:D8:B3:E1:30:58:39:35:41\r\na=setup:actpass\r\na=mid:data\r\na=sctpmap:5000 webrtc-datachannel 1024\r\n"},"type":"data","label":"dc_d29iaxm9wi","connectionId":"dc_d29iaxm9wi","reliable":false,"serialization":"binary","browser":"Chrome"}`,
	`{"candidate":{"candidate":"candidate:842163049 1 udp 1677729535 195.174.144.24 56801 typ srflx raddr 0.0.0.0 rport 0 generation 0 ufrag 0pH9 network-cost 50","sdpMid":"data","sdpMLineIndex":0},"type":"data","connectionId":"dc_d29iaxm9wi"}`,
	`{"msg":"ID is taken"}`,
}

func TestParseWebRTCMessage(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		msg          *WebRTCSignalMessage
		checkPayload bool
		wantErr      bool
	}{
		{
			name:         "non-utf8 encoded messages",
			message:      string([]byte{0x00}),
			msg:          nil,
			checkPayload: false,
			wantErr:      true,
		},
		{
			name:    "utf8 encoded messages",
			message: msgs[0],
			msg: &WebRTCSignalMessage{
				Type: "ANSWER",
				Src:  "dest-peer-id",
				Dst:  "someid",
			},
			checkPayload: false,
			wantErr:      false,
		},
		{
			name:         "invalid message",
			message:      "naberpamps",
			msg:          nil,
			checkPayload: false,
			wantErr:      true,
		},
		{
			name:    "parse answer message",
			message: msgs[0],
			msg: &WebRTCSignalMessage{
				Type: "ANSWER",
				Src:  "dest-peer-id",
				Dst:  "someid",
			},
			checkPayload: false,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := ParseWebRTCSignalMessage(tt.message)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWebRTCMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// in case they are both nil
			if sm == tt.msg {
				return
			}

			if !tt.checkPayload {
				sm.Payload = nil
				tt.msg.Payload = nil
			}

			smJSON, err := json.Marshal(sm)
			if err != nil {
				t.Errorf("json.Marshal(sm) err = %v", err)
			}

			msgJSON, err := json.Marshal(tt.msg)
			if err != nil {
				t.Errorf("json.Marshal(tt.msg) err = %v", err)
			}

			if string(smJSON) != string(msgJSON) {
				t.Errorf("string(smJSON) != string(msgJSON) smJSON = %v, want %v", string(smJSON), string(msgJSON))
			}
		})
	}
}

func TestWebRTCSignalMessage_ParsePayload(t *testing.T) {
	tests := []struct {
		name    string
		payload json.RawMessage
		want    *Payload
		wantErr bool
	}{
		{
			name:    "",
			payload: []byte(payloads[0]),
			want: &Payload{
				Type:         s("data"),
				ConnectionID: s("dc_d29iaxm9wi"),
				Browser:      s("Chrome"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &WebRTCSignalMessage{
				Payload: tt.payload,
			}
			got, err := w.ParsePayload()
			if (err != nil) != tt.wantErr {
				t.Errorf("WebRTCSignalMessage.ParsePayload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Errorf("json.Marshal(sm) err = %v", err)
			}

			wantJSON, err := json.Marshal(tt.want)
			if err != nil {
				t.Errorf("json.Marshal(tt.want) err = %v", err)
			}

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("string(gotJSON) != string(wantJSON) gotJSON = %v, want %v", string(gotJSON), string(wantJSON))
			}
		})
	}
}

func s(str string) *string { return &str }
