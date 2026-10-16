/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { JSX } from "react";
import type { PagingChainMember } from "../types";
import { ADD_TO_PROFILE_HINT, phoneChipState } from "../utils/pagingPhone";

/** What a reader may do about a person's alert number; the tab owns it. */
export interface PagingPhoneControls {
  edit: (member: PagingChainMember) => void;
  remove: (member: PagingChainMember) => void;
  test: (member: PagingChainMember) => void;
  /** Whether a test call to this person is placed and its result awaited. */
  calling: (member: PagingChainMember) => boolean;
}

/** "Calling…" while a test call's result is awaited. */
export const CALLING_LABEL = "Calling… result in about a minute";

function PhoneIcon(): JSX.Element {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <path d="M22 16.9v3a2 2 0 0 1-2.2 2 19.8 19.8 0 0 1-8.6-3.1 19.5 19.5 0 0 1-6-6A19.8 19.8 0 0 1 2.1 4.2 2 2 0 0 1 4.1 2h3a2 2 0 0 1 2 1.7c.1.9.4 1.8.7 2.7a2 2 0 0 1-.5 2.1L8 9.8a16 16 0 0 0 6 6l1.3-1.3a2 2 0 0 1 2.1-.4c.9.3 1.8.6 2.7.7a2 2 0 0 1 1.7 2z" />
    </svg>
  );
}

/**
 * The number Case Alerts would call for one person, shown only when there is
 * something to act on ("No number", an alert number that failed its test,
 * ...): a number on the person's own profile is the normal case and shows
 * nothing. The colour only repeats the words, and only the masked number is
 * ever shown. A number is added on the person's profile, not here, so a
 * missing one says where to add it. A paging-only number set before that rule
 * can still be test-called or removed by a reader who may.
 */
export default function PhoneChip({
  member,
  controls,
}: {
  member: PagingChainMember;
  controls?: PagingPhoneControls;
}): JSX.Element | null {
  const st = phoneChipState(member);
  const name = member.name || member.email;
  const can = Boolean(controls) && member.canEditPhone === true;
  const calling = can && controls ? controls.calling(member) : false;
  const onProfile = member.hasProfilePhone === true;

  // A number on the profile needs nothing said about it; only an older
  // paging-only number's actions are still offered beside it.
  if (onProfile && !(can && member.pagingPhone)) return null;

  return (
    <span className="cp-phone">
      {onProfile ? null : (
        <span className={`cp-ph ${st.tone}`} aria-live="polite">
          <PhoneIcon />
          <span>
            {st.label}
            {st.masked ? <span className="cp-mask"> · {st.masked}</span> : null}
          </span>
        </span>
      )}
      {!member.pagingPhone && member.hasProfilePhone === false ? (
        <span className="cp-fixnote">{ADD_TO_PROFILE_HINT}</span>
      ) : null}
      {can && controls && member.pagingPhone ? (
        <span className="cp-phact">
          {member.pagingPhone ? (
            <>
              <button
                type="button"
                className="btn ghost sm"
                aria-label={`Remove ${name}'s alert number`}
                onClick={() => controls.remove(member)}
              >
                Remove
              </button>
              <button
                type="button"
                className="btn ghost sm"
                aria-label={calling ? `${CALLING_LABEL} (${name})` : `Test call ${name}'s alert number`}
                aria-disabled={calling || undefined}
                onClick={() => {
                  if (!calling) controls.test(member);
                }}
              >
                {calling ? CALLING_LABEL : "Test call"}
              </button>
            </>
          ) : null}
        </span>
      ) : null}
    </span>
  );
}
