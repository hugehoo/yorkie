/*
 * Copyright 2021 The Yorkie Authors. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package backend

import (
	"fmt"
	"time"
)

// Config is the configuration for creating a Backend instance.
type Config struct {
	// AdminUser is the name of the default admin user who has full permissions.
	// Set once on first-run. Default is "admin".
	AdminUser string `yaml:"AdminUser"`

	// AdminPassword is the password of the default admin. Default is "admin".
	AdminPassword string `yaml:"AdminPassword"`

	// SecretKey is the secret key for signing authentication tokens.
	SecretKey string `yaml:"SecretKey"`

	// AdminTokenDuration is the duration of the admin token. Default is "7d".
	AdminTokenDuration string `yaml:"AdminTokenDuration"`

	// UseDefaultProject is whether to use the default project. Even if public
	// key is not provided from the client, the default project will be used. If
	// we are using server as single-tenant mode, this should be set to true.
	UseDefaultProject bool `yaml:"UseDefaultProject"`

	// SnapshotThreshold is the threshold that determines if changes should be
	// sent with snapshot when the number of changes is greater than this value.
	SnapshotThreshold int64 `yaml:"SnapshotThreshold"`

	// SnapshotInterval is the interval of changes to create a snapshot.
	SnapshotInterval int64 `yaml:"SnapshotInterval"`

	// AuthWebhookMaxRetries is the max count that retries the authorization webhook.
	AuthWebhookMaxRetries uint64 `yaml:"AuthWebhookMaxRetries"`

	// AuthWebhookMaxWaitInterval is the max interval that waits before retrying the authorization webhook.
	AuthWebhookMaxWaitInterval string `yaml:"AuthWebhookMaxWaitInterval"`

	// AuthWebhookCacheSize is the cache size of the authorization webhook.
	AuthWebhookCacheSize int `yaml:"AuthWebhookCacheSize"`

	// AuthWebhookCacheAuthTTL is the TTL value to set when caching the authorized result.
	AuthWebhookCacheAuthTTL string `yaml:"AuthWebhookCacheAuthTTL"`

	// AuthWebhookCacheUnauthTTL is the TTL value to set when caching the unauthorized result.
	AuthWebhookCacheUnauthTTL string `yaml:"AuthWebhookCacheUnauthTTL"`
}

// Validate validates this config.
func (c *Config) Validate() error {
	if _, err := time.ParseDuration(c.AuthWebhookMaxWaitInterval); err != nil {
		return fmt.Errorf(
			`invalid argument "%s" for "--auth-webhook-max-wait-interval" flag: %w`,
			c.AuthWebhookMaxWaitInterval,
			err,
		)
	}

	if _, err := time.ParseDuration(c.AuthWebhookCacheAuthTTL); err != nil {
		return fmt.Errorf(
			`invalid argument "%s" for "--auth-webhook-cache-auth-ttl" flag: %w`,
			c.AuthWebhookCacheAuthTTL,
			err,
		)
	}

	if _, err := time.ParseDuration(c.AuthWebhookCacheUnauthTTL); err != nil {
		return fmt.Errorf(
			`invalid argument "%s" for "--auth-webhook-cache-unauth-ttl" flag: %w`,
			c.AuthWebhookCacheUnauthTTL,
			err,
		)
	}

	return nil
}

// ParseAdminTokenDuration returns admin token duration.
func (c *Config) ParseAdminTokenDuration() time.Duration {
	result, err := time.ParseDuration(c.AdminTokenDuration)
	if err != nil {
		panic(err)
	}

	return result
}

// ParseAuthWebhookMaxWaitInterval returns max wait interval.
func (c *Config) ParseAuthWebhookMaxWaitInterval() time.Duration {
	result, err := time.ParseDuration(c.AuthWebhookMaxWaitInterval)
	if err != nil {
		panic(err)
	}

	return result
}

// ParseAuthWebhookCacheAuthTTL returns TTL for authorized cache.
func (c *Config) ParseAuthWebhookCacheAuthTTL() time.Duration {
	result, err := time.ParseDuration(c.AuthWebhookCacheAuthTTL)
	if err != nil {
		panic(err)
	}

	return result
}

// ParseAuthWebhookCacheUnauthTTL returns TTL for unauthorized cache.
func (c *Config) ParseAuthWebhookCacheUnauthTTL() time.Duration {
	result, err := time.ParseDuration(c.AuthWebhookCacheUnauthTTL)
	if err != nil {
		panic(err)
	}

	return result
}
