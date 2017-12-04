// @flow
import * as I from 'immutable'
import * as ChatTypes from './types/chat'
import * as Types from './types/teams'
import {userIsInTeam} from './selectors'
import * as RPCTypes from './types/flow-types'
import invert from 'lodash/invert'

import type {Service} from './types/search'
import {type TypedState} from './reducer'

export const teamRoleTypes = ['reader', 'writer', 'admin', 'owner']

export const makeChannelInfo: I.RecordFactory<Types._ChannelInfo> = I.Record({
  channelname: null,
  description: null,
  participants: I.Set(),
})

export const makeMemberInfo: I.RecordFactory<Types._MemberInfo> = I.Record({
  type: null,
  username: '',
})

export const makeInviteInfo: I.RecordFactory<Types._InviteInfo> = I.Record({
  email: '',
  name: '',
  role: 'writer',
  username: '',
  id: '',
})

export const makeRequestInfo: I.RecordFactory<Types._RequestInfo> = I.Record({
  username: '',
})

export const teamRoleByEnum = invert(RPCTypes.teamsTeamRole)

export const typeToLabel: Types.TypeMap = {
  admin: 'Admin',
  owner: 'Owner',
  reader: 'Reader',
  writer: 'Writer',
}

export const makeState: I.RecordFactory<Types._State> = I.Record({
  convIDToChannelInfo: I.Map(),
  loaded: false,
  sawChatBanner: false,
  teamNameToConvIDs: I.Map(),
  teamNameToInvites: I.Map(),
  teamNameToLoadingInvites: I.Map(),
  teamNameToLoading: I.Map(),
  teamNameToMemberUsernames: I.Map(),
  teamNameToImplicitAdminUsernames: I.Map(),
  teamNameToMembers: I.Map(),
  teamNameToRequests: I.Map(),
  teamNameToRole: I.Map(),
  teamNameToTeamSettings: I.Map(),
  teamNameToPublicitySettings: I.Map(),
  teammembercounts: I.Map(),
  newTeams: I.Set(),
  newTeamRequests: I.List(),
  teamnames: I.Set(),
})

const userIsInTeamHelper = (state: TypedState, username: string, service: Service, teamname: string) =>
  service === 'Keybase' ? userIsInTeam(state, teamname, username) : false

const getConvIdsFromTeamName = (state: TypedState, teamname: string): I.Set<string> =>
  state.entities.teams.teamNameToConvIDs.get(teamname, I.Set())

const getTeamNameFromConvID = (state: TypedState, conversationIDKey: ChatTypes.ConversationIDKey) =>
  state.entities.teams.teamNameToConvIDs.findKey(i => i.has(conversationIDKey))

const getChannelNameFromConvID = (state: TypedState, conversationIDKey: ChatTypes.ConversationIDKey) =>
  state.entities.teams.convIDToChannelInfo.getIn([conversationIDKey, 'channelname'], null)

const getTopicFromConvID = (state: TypedState, conversationIDKey: ChatTypes.ConversationIDKey) =>
  state.entities.teams.convIDToChannelInfo.getIn([conversationIDKey, 'description'], null)

const getRole = (state: TypedState, teamname: Types.Teamname): ?Types.TeamRoleType =>
  state.entities.getIn(['teams', 'teamNameToRole', teamname], null)

const isAdmin = (type: ?Types.TeamRoleType) => type === 'admin'
const isOwner = (type: ?Types.TeamRoleType) => type === 'owner'

export {
  getConvIdsFromTeamName,
  getRole,
  userIsInTeamHelper,
  getTeamNameFromConvID,
  getChannelNameFromConvID,
  getTopicFromConvID,
  isAdmin,
  isOwner,
}
