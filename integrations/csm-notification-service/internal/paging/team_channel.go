// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package paging

// An SRE team's own channel is set on the Team Schedule's Alert Settings tab
// (Team routing) and reaches this service as LadderConfig.TeamChannels -- never
// from the file. It can only narrow the ladder's channel: the file's channel
// is the most any team's alerts may do, so a team can drop calls or chat, or
// go quiet (log), but never add a channel its ladder does not use. The safety
// caps apply as before.

// narrowChannel is what a team may use under its ladder's channel: only what
// both allow. An empty team channel is the ladder's.
func narrowChannel(ladder, team Channel) Channel {
	if team == "" {
		return ladder
	}
	chat := ladder.Uses(ChannelChat) && team.Uses(ChannelChat)
	call := ladder.Uses(ChannelCall) && team.Uses(ChannelCall)
	switch {
	case chat && call:
		return ChannelBoth
	case chat:
		return ChannelChat
	case call:
		return ChannelCall
	}
	return ChannelLog
}

// channelFor is how one incident's alerts go out: its own SRE team's channel,
// within the ladder's. An incident with no SRE team of its own (a default
// group, a customer case at S0) uses the ladder's.
func (e *Engine) channelFor(t Trigger) Channel {
	if len(e.cfg.Ladder.TeamChannels) == 0 {
		return e.cfg.Channel
	}
	if c, ok := e.cfg.Ladder.TeamChannels[e.sreTeamKey(t.Team)]; ok {
		return narrowChannel(e.cfg.Channel, c)
	}
	return e.cfg.Channel
}

// withChannel keeps the notifiers a narrowed channel uses. A team set to log
// reaches nobody: its rungs are written down, as a log ladder's are.
func withChannel(notifiers []notifier, ladder, ch Channel) []notifier {
	if ch == ladder {
		return notifiers
	}
	if ch == ChannelLog {
		return []notifier{logNotifier{}}
	}
	out := []notifier{}
	for _, n := range notifiers {
		if ch.Uses(n.Channel()) {
			out = append(out, n)
		}
	}
	return out
}
