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

import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import {
  INCIDENTS_ENDPOINT,
  INCIDENTS_SEARCH_ENDPOINT,
  INCIDENT_COMMENTS_SEARCH_ENDPOINT,
  INCIDENT_ENDPOINT,
} from "@config/endpoints";
import type {
  CaseCommentSearchResponseDto,
  IncidentCreatePayloadDto,
  IncidentCreateResponseDto,
  IncidentDetailDto,
  IncidentSearchPayloadDto,
  IncidentSearchResponseDto,
} from "@src/types";
import { toComment, toIncidentDetail, toIncidentSummary, type Comment, type IncidentSummary } from "@src/types";
import apiClient from "./apiClient";

export interface IncidentSearchResult {
  items: IncidentSummary[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

const searchIncidents = async (payload: IncidentSearchPayloadDto = {}): Promise<IncidentSearchResult> => {
  const { data } = await apiClient.post<IncidentSearchResponseDto>(INCIDENTS_SEARCH_ENDPOINT, payload);
  const items = data.incidents.map(toIncidentSummary);
  // The search response has no `hasMore` field (unlike /cases/search) — derive it from
  // offset/total, same defensive pattern as changeRequests.ts's searchChangeRequests.
  return {
    items,
    total: data.total,
    limit: data.limit,
    offset: data.offset,
    hasMore: items.length > 0 && data.offset + items.length < data.total,
  };
};

const getIncident = async (id: string) => {
  const { data } = await apiClient.get<IncidentDetailDto>(INCIDENT_ENDPOINT(id));
  return toIncidentDetail(data);
};

// Matches cases.ts's COMMENTS_PAGE_LIMIT — the live entity service rejects the openapi-declared
// 100 max with a 400. Reuses the exact same CaseCommentSearchResponseDto/toComment shape as case
// comments: incidents' `/comments/search` endpoint returns the identical generic comment record,
// just under a different reference path.
const COMMENTS_PAGE_LIMIT = 50;

const getIncidentComments = async (id: string): Promise<Comment[]> => {
  const { data } = await apiClient.post<CaseCommentSearchResponseDto>(INCIDENT_COMMENTS_SEARCH_ENDPOINT(id), {
    pagination: { limit: COMMENTS_PAGE_LIMIT },
  });
  return data.comments.map(toComment);
};

const createIncident = async (payload: IncidentCreatePayloadDto): Promise<IncidentCreateResponseDto> => {
  const { data } = await apiClient.post<IncidentCreateResponseDto>(INCIDENTS_ENDPOINT, payload);
  return data;
};

const INCIDENT_PAGE_LIMIT = 20;

export const incidents = {
  // Most-recently-updated first, mirroring cases.ts's infinite (updatedOn desc) — not left to the
  // backend's default order.
  infinite: (payload: Omit<IncidentSearchPayloadDto, "pagination" | "sortBy"> = {}) =>
    infiniteQueryOptions({
      queryKey: ["incidents", "infinite", payload],
      queryFn: ({ pageParam }) =>
        searchIncidents({
          ...payload,
          sortBy: { field: "updatedOn", order: "desc" },
          pagination: { offset: pageParam, limit: INCIDENT_PAGE_LIMIT },
        }),
      initialPageParam: 0,
      getNextPageParam: (lastPage) => (lastPage.hasMore ? lastPage.offset + lastPage.limit : undefined),
    }),

  get: (id: string) =>
    queryOptions({
      queryKey: ["incident", id],
      queryFn: () => getIncident(id),
    }),

  comments: (id: string) =>
    queryOptions({
      queryKey: ["incident", id, "comments"],
      queryFn: () => getIncidentComments(id),
    }),

  create: createIncident,
};
