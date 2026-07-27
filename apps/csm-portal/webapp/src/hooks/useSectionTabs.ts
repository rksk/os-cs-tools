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

/**
 * Drives a section's second-level tab strip from the navigation tree, so the
 * tabs a deployment is allowed to see are decided in one place
 * (`CSM_PORTAL_FEATURE_OVERRIDES`) rather than per page.
 *
 * Two flavours, matching the two tab idioms in the app: `useQueryTabs` for
 * sections that keep the selection in `?tab=` (Operations, Security Center) and
 * `useRouteTabs` for sections whose tabs are child routes (Customers,
 * Settings).
 */

import { useLocation, useSearchParams } from "react-router";
import {
  type CsmNavNode,
  navNodeById,
  navNodePath,
} from "@config/csmNavItems";
import {
  type FeatureState,
  enabledNavChildren,
  featureState,
  visibleNavChildren,
} from "@config/featureFlags";
import { useNavTransition } from "@hooks/useNavTransition";

/** One rendered tab: the nav node plus the state that decides how it looks. */
export interface SectionTab {
  /** Value the `<Tabs>` strip is keyed by. */
  key: string;
  label: string;
  state: FeatureState;
  node: CsmNavNode;
}

export interface SectionTabsState {
  /** Tabs to render — enabled and WIP ones; hidden tabs are gone entirely. */
  tabs: SectionTab[];
  /** Key of the selected tab, or `""` when the section has no visible tabs. */
  activeKey: string;
  select: (key: string) => void;
}

/**
 * Picks the tab to show. Honours the caller's request only when that tab is
 * usable, so a link to a tab this deployment marked WIP or hidden lands on the
 * first working tab instead of on a dead panel. Falls back to the first visible
 * tab when nothing is enabled, so the strip still renders something.
 */
function resolveActiveKey(tabs: SectionTab[], requested: string | null): string {
  const requestedTab = requested
    ? tabs.find((tab) => tab.key === requested)
    : undefined;
  if (requestedTab?.state === "enabled") return requestedTab.key;
  return (
    tabs.find((tab) => tab.state === "enabled")?.key ?? tabs[0]?.key ?? ""
  );
}

function tabsFor(
  sectionId: string,
  keyOf: (node: CsmNavNode) => string,
): SectionTab[] {
  const section = navNodeById(sectionId);
  if (!section) return [];
  return visibleNavChildren(section).map((node) => ({
    key: keyOf(node),
    label: node.label,
    state: featureState(node.id),
    node,
  }));
}

/** Tab strip for a section that keeps its selection in the `?tab=` query. */
export function useQueryTabs(sectionId: string): SectionTabsState {
  const [searchParams, setSearchParams] = useSearchParams();
  // Query tabs declare their `?tab=` value on the nav node; a node without one
  // can't be selected, so it is keyed by id and simply never matches.
  const tabs = tabsFor(sectionId, (node) => node.tab ?? node.id);
  const activeKey = resolveActiveKey(tabs, searchParams.get("tab"));

  return {
    tabs,
    activeKey,
    select: (key: string) =>
      setSearchParams((prev) => {
        prev.set("tab", key);
        return prev;
      }),
  };
}

/** Tab strip for a section whose tabs are child routes. */
export function useRouteTabs(sectionId: string): SectionTabsState {
  const { pathname } = useLocation();
  const navigate = useNavTransition();
  const tabs = tabsFor(sectionId, (node) => navNodePath(node));

  const current = tabs.find(
    (tab) => pathname === tab.key || pathname.startsWith(`${tab.key}/`),
  );
  const activeKey = resolveActiveKey(tabs, current?.key ?? null);

  return {
    tabs,
    activeKey,
    select: (key: string) => void navigate(key),
  };
}

/**
 * Where a section's index route should land: its first usable tab. Returns
 * `undefined` when every tab is restricted, leaving the caller to decide.
 */
export function firstEnabledTabHref(sectionId: string): string | undefined {
  const section = navNodeById(sectionId);
  return section ? enabledNavChildren(section)[0]?.href : undefined;
}
