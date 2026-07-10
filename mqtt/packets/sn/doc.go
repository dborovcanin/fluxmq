// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

// Package sn implements MQTT-SN 1.2 packet encoding and decoding.
//
// The package is intentionally wire-format only. It does not allocate topic
// IDs, track retransmission state, manage sleeping clients, or translate
// packets into broker operations.
package sn
