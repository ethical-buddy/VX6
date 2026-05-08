package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type rtcSignal struct {
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	Type      string `json:"type"` // offer|answer|candidate|hangup
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
	MID       string `json:"mid,omitempty"`
	MLine     uint16 `json:"mline,omitempty"`
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

type rtcSession struct {
	mu        sync.Mutex
	peerID    string
	pc        *webrtc.PeerConnection
	videoSSRC uint32
	stop      context.CancelFunc
	lastSigID string
}

func rtcKey(nodeID string) string {
	return "vx6chat/rtc/" + nodeID
}

func (s *state) ensureRTCSession(peer peerContact) (*rtcSession, error) {
	s.mu.Lock()
	if s.local.ActiveGroups == nil {
		s.local.ActiveGroups = map[string]groupRoom{}
	}
	s.mu.Unlock()

	if srt, ok := s.rtcLoad(peer.NodeID); ok {
		return srt, nil
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, err
	}
	ss := &rtcSession{
		peerID:    peer.NodeID,
		pc:        pc,
		videoSSRC: 424242,
	}
	ctx, cancel := context.WithCancel(context.Background())
	ss.stop = cancel

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		_ = pc.Close()
		cancel()
		return nil, err
	}
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeVP8,
		ClockRate: 90000,
	}, "video", "vx6")
	if err == nil {
		_, _ = pc.AddTrack(videoTrack)
		go s.pushSyntheticRTP(ctx, videoTrack, ss.videoSSRC)
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		_ = s.publishRTCSignal(peer.NodeID, rtcSignal{
			FromID:    s.id.NodeID,
			ToID:      peer.NodeID,
			Type:      "candidate",
			Candidate: init.Candidate,
			MID:       derefString(init.SDPMid),
			MLine:     derefUint16(init.SDPMLineIndex),
			ID:        fmt.Sprintf("rtc-%d", time.Now().UnixNano()),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
			cancel()
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		// Consume inbound RTP to keep receiver path active.
		go func() {
			for {
				_, _, err := track.ReadRTP()
				if err != nil {
					return
				}
			}
		}()
	})

	s.rtcStore(peer.NodeID, ss)
	return ss, nil
}

func (s *state) pushSyntheticRTP(ctx context.Context, track *webrtc.TrackLocalStaticRTP, ssrc uint32) {
	tk := time.NewTicker(33 * time.Millisecond)
	defer tk.Stop()
	var seq uint16
	var ts uint32
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			seq++
			ts += 3000
			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    96,
					SequenceNumber: seq,
					Timestamp:      ts,
					SSRC:           ssrc,
					Marker:         true,
				},
				// Small synthetic payload keeps RTP path alive; replace with encoder output later.
				Payload: []byte{0x90, 0x90, 0x90, 0x90},
			}
			_ = track.WriteRTP(pkt)
		}
	}
}

func (s *state) publishRTCSignal(to string, sig rtcSignal) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.client.DHTPut(ctx, rtcKey(to), marshalJSON(sig))
}

func (s *state) initiateWebRTCCall(peer peerContact) error {
	ss, err := s.ensureRTCSession(peer)
	if err != nil {
		return err
	}
	offer, err := ss.pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := ss.pc.SetLocalDescription(offer); err != nil {
		return err
	}
	return s.publishRTCSignal(peer.NodeID, rtcSignal{
		FromID:    s.id.NodeID,
		ToID:      peer.NodeID,
		Type:      "offer",
		SDP:       offer.SDP,
		ID:        fmt.Sprintf("rtc-%d", time.Now().UnixNano()),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *state) pollRTCSignals() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, rtcKey(s.id.NodeID))
	if err != nil || len(raw) == 0 {
		return nil
	}
	var sig rtcSignal
	if err := json.Unmarshal(raw, &sig); err != nil {
		return nil
	}
	if sig.ToID != s.id.NodeID || sig.FromID == "" || sig.ID == "" {
		return nil
	}
	if s.rtcSeen(sig.FromID, sig.ID) {
		return nil
	}
	peer := s.findContactByID(sig.FromID)
	if peer.NodeID == "" {
		return nil
	}
	ss, err := s.ensureRTCSession(peer)
	if err != nil {
		return nil
	}
	switch sig.Type {
	case "offer":
		_ = ss.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sig.SDP})
		answer, err := ss.pc.CreateAnswer(nil)
		if err == nil {
			_ = ss.pc.SetLocalDescription(answer)
			_ = s.publishRTCSignal(peer.NodeID, rtcSignal{
				FromID: s.id.NodeID, ToID: peer.NodeID, Type: "answer", SDP: answer.SDP,
				ID: fmt.Sprintf("rtc-%d", time.Now().UnixNano()), CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
	case "answer":
		_ = ss.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sig.SDP})
	case "candidate":
		cand := webrtc.ICECandidateInit{
			Candidate: sig.Candidate,
		}
		if sig.MID != "" {
			m := sig.MID
			cand.SDPMid = &m
		}
		if sig.MLine > 0 {
			ml := sig.MLine
			cand.SDPMLineIndex = &ml
		}
		_ = ss.pc.AddICECandidate(cand)
	case "hangup":
		if ss.stop != nil {
			ss.stop()
		}
		_ = ss.pc.Close()
		s.rtcDelete(peer.NodeID)
	}
	return nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefUint16(v *uint16) uint16 {
	if v == nil {
		return 0
	}
	return *v
}
