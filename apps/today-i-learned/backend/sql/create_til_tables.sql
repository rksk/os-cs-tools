-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
--
-- WSO2 LLC. licenses this file to you under the Apache License,
-- Version 2.0 (the "License"); you may not use this file except
-- in compliance with the License.
-- You may obtain a copy of the License at
--
-- http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing,
-- software distributed under the License is distributed on an
-- "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
-- KIND, either express or implied.  See the License for the
-- specific language governing permissions and limitations
-- under the License.

-- Creates the tables til-backend needs. Run it once against the database
-- before starting the service; it is safe to run again. The service never
-- creates tables itself, and stops at startup if til_entry is missing.
--
--   mysql -h <host> -u <user> -p <database> < sql/create_til_tables.sql

-- One row per TIL entry.

CREATE TABLE IF NOT EXISTS til_entry (
    id            CHAR(36)     NOT NULL PRIMARY KEY,
    title         VARCHAR(100) NOT NULL,
    -- The byline as the author shared it, and the verified email of the
    -- person who shared it. Only the email decides who may edit or delete.
    author_name   VARCHAR(200) NOT NULL,
    author_email  VARCHAR(255) NOT NULL,
    -- Customer, Partner, Internal or Other. source_detail names the customer
    -- or partner, or explains Other; it is NULL for Internal.
    source        VARCHAR(20)  NOT NULL,
    source_detail VARCHAR(200) NULL,
    -- The learning as sanitised HTML, and its plain text. Search and the
    -- length limit read the plain text, so markup never matches a search.
    content_html  MEDIUMTEXT   NOT NULL,
    content_text  TEXT         NOT NULL,
    -- UTC. updated_on stays NULL until the entry is first edited.
    created_on    DATETIME(6)  NOT NULL,
    updated_on    DATETIME(6)  NULL,
    -- UTC. When Novera accepted the entry's Google Chat announcement; NULL
    -- while it hasn't, so a moderator can send it again.
    notified_at   DATETIME(6)  NULL,
    INDEX idx_til_entry_created (created_on DESC, id DESC),
    INDEX idx_til_entry_author_email (author_email),
    FULLTEXT INDEX idx_til_entry_search (title, content_text, author_name, source_detail)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
