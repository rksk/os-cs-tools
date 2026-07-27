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

import {
  CONFIGURATION_ITEMS_SEARCH_ENDPOINT,
  GROUPS_SEARCH_ENDPOINT,
  IT_SERVICES_SEARCH_ENDPOINT,
  SERVICE_OFFERINGS_SEARCH_ENDPOINT,
  USERS_SEARCH_ENDPOINT,
} from "@config/endpoints";
import type {
  ConfigurationItemSearchResponseDto,
  GroupSearchResponseDto,
  ItServiceSearchResponseDto,
  ServiceOfferingSearchResponseDto,
  UserSearchResponseDto,
} from "@src/types";
import { toAdminUser } from "@src/types";
import apiClient from "./apiClient";

// Type-ahead pickers for the change-request create form's "More options" section (Service,
// Service offering, Configuration item, Assignment group, Assigned to, Requested by). Mirrors the
// webapp's useSearchGroups/useSearchItServices/useSearchServiceOfferings/
// useSearchConfigurationItems/useSearchUsersByName — a single page of matches is plenty for a
// type-ahead field, so unlike deployments.ts/catalogs.ts these don't page through the full result
// set.
const LOOKUP_SEARCH_LIMIT = 20;

export interface EntityOption {
  id: string;
  label: string;
}

const searchGroups = async (query: string): Promise<EntityOption[]> => {
  const { data } = await apiClient.post<GroupSearchResponseDto>(GROUPS_SEARCH_ENDPOINT, {
    filters: { searchQuery: query },
    pagination: { offset: 0, limit: LOOKUP_SEARCH_LIMIT },
  });
  return (data.groups ?? []).map((g) => ({ id: g.id, label: g.name }));
};

const searchItServices = async (query: string): Promise<EntityOption[]> => {
  const { data } = await apiClient.post<ItServiceSearchResponseDto>(IT_SERVICES_SEARCH_ENDPOINT, {
    filters: { searchQuery: query },
    pagination: { offset: 0, limit: LOOKUP_SEARCH_LIMIT },
  });
  return (data.services ?? []).map((s) => ({ id: s.id, label: s.name || s.id }));
};

// Narrows to offerings under `serviceId` when one is given, matching how the ServiceNow form
// itself scopes this field once a Service is picked.
const searchServiceOfferings = async (query: string, serviceId?: string): Promise<EntityOption[]> => {
  const { data } = await apiClient.post<ServiceOfferingSearchResponseDto>(SERVICE_OFFERINGS_SEARCH_ENDPOINT, {
    filters: { searchQuery: query, ...(serviceId && { serviceIds: [serviceId] }) },
    pagination: { offset: 0, limit: LOOKUP_SEARCH_LIMIT },
  });
  return (data.serviceOfferings ?? []).map((o) => ({ id: o.id, label: o.name }));
};

const searchConfigurationItems = async (query: string): Promise<EntityOption[]> => {
  const { data } = await apiClient.post<ConfigurationItemSearchResponseDto>(CONFIGURATION_ITEMS_SEARCH_ENDPOINT, {
    filters: { searchQuery: query },
    pagination: { offset: 0, limit: LOOKUP_SEARCH_LIMIT },
  });
  return (data.configurationItems ?? []).map((ci) => ({ id: ci.id, label: ci.name || ci.id }));
};

// Unlike adminUsers.ts's `search` (scoped to active internal approver roles, for the time-card
// approver picker), "Requested by"/"Assigned to" on a change request can be anyone — no
// role/active filter, same as the webapp's useSearchUsersByName.
const searchUsersByName = async (query: string): Promise<EntityOption[]> => {
  const { data } = await apiClient.post<UserSearchResponseDto>(USERS_SEARCH_ENDPOINT, {
    filters: { searchQuery: query },
    pagination: { offset: 0, limit: LOOKUP_SEARCH_LIMIT },
  });
  return data.users.map((u) => {
    const user = toAdminUser(u);
    return { id: user.id, label: user.name || user.email };
  });
};

// Plain async functions rather than queryOptions() factories — AsyncEntitySelect builds its own
// useQuery around whichever of these it's given (see that component for why), so these just need
// to satisfy its `search: (query, extra?) => Promise<EntityOption[]>` contract.
export const changeRequestLookups = {
  groups: searchGroups,
  itServices: searchItServices,
  serviceOfferings: searchServiceOfferings,
  configurationItems: searchConfigurationItems,
  users: searchUsersByName,
};
