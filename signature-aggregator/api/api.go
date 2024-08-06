// Copyright (C) 2024, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/awm-relayer/signature-aggregator/aggregator"
	"github.com/ava-labs/awm-relayer/types"
	"github.com/ava-labs/awm-relayer/utils"
	"go.uber.org/zap"
)

const (
	APIPath          = "/aggregate-signatures"
	QuorumPercentage = 67
)

// Defines a request interface for signature aggregation for a raw unsigned message.
type AggregateSignatureRequest struct {
	// Required. hex-encoded message, optionally prefixed with "0x".
	UnsignedMessage string `json:"unsigned-message"`
	// Optional hex or cb58 encoded signing subnet ID. If omitted will default to the subnetID of the source blockchain
	SigningSubnetID string `json:"signing-subnet-id"`
	// Optional. Integer from 0 to 100 representing the percentage of the quorum that is required to sign the message
	// defaults to 67 if omitted.
	QuorumPercentage uint64 `json:"quorum-percentage"`
}

type AggregateSignatureResponse struct {
	// hex encoding of the signature
	SignedMessage string `json:"signed-message"`
}

func HandleAggregateSignaturesByRawMsgRequest(
	logger logging.Logger,
	signatureAggregator *aggregator.SignatureAggregator,
) {
	http.Handle(APIPath, signatureAggregationAPIHandler(logger, signatureAggregator))
}

func writeJSONError(
	logger logging.Logger,
	w http.ResponseWriter,
	errorMsg string,
) {
	resp, err := json.Marshal(struct{ error string }{error: errorMsg})
	if err != nil {
		msg := "Error marshalling JSON error response"
		logger.Error(msg, zap.Error(err))
		resp = []byte(msg)
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(resp)
	if err != nil {
		logger.Error("Error writing error response", zap.Error(err))
	}
}

func signatureAggregationAPIHandler(logger logging.Logger, aggregator *aggregator.SignatureAggregator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req AggregateSignatureRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			msg := "Could not decode request body"
			logger.Warn(msg, zap.Error(err))
			writeJSONError(logger, w, msg)
			return
		}
		var decodedMessage []byte
		decodedMessage, err = hex.DecodeString(
			strings.TrimPrefix(req.UnsignedMessage, "0x"),
		)
		if err != nil {
			msg := "Could not decode message"
			logger.Warn(
				msg,
				zap.String("msg", req.UnsignedMessage),
				zap.Error(err),
			)
			writeJSONError(logger, w, msg)
			return
		}
		unsignedMessage, err := types.UnpackWarpMessage(decodedMessage)
		if err != nil {
			msg := "Error unpacking warp message"
			logger.Warn(msg, zap.Error(err))
			writeJSONError(logger, w, msg)
			return
		}
		quorumPercentage := req.QuorumPercentage
		if quorumPercentage == 0 {
			quorumPercentage = QuorumPercentage
		} else if req.QuorumPercentage > 100 {
			msg := "Invalid quorum number"
			logger.Warn(msg, zap.Uint64("quorum-num", req.QuorumPercentage))
			writeJSONError(logger, w, msg)
			return
		}
		var signingSubnetID ids.ID
		if req.SigningSubnetID != "" {
			signingSubnetID, err = utils.HexOrCB58ToID(
				req.SigningSubnetID,
			)
			if err != nil {
				msg := "Error parsing signing subnet ID"
				logger.Warn(
					msg,
					zap.Error(err),
					zap.String("input", req.SigningSubnetID),
				)
				writeJSONError(logger, w, msg)
			}
		}

		signedMessage, err := aggregator.AggregateSignaturesAppRequest(
			unsignedMessage,
			signingSubnetID,
			quorumPercentage,
		)
		if err != nil {
			msg := "Failed to aggregate signatures"
			logger.Warn(msg, zap.Error(err))
			writeJSONError(logger, w, msg)
		}
		resp, err := json.Marshal(
			AggregateSignatureResponse{
				SignedMessage: hex.EncodeToString(
					signedMessage.Bytes(),
				),
			},
		)

		if err != nil {
			msg := "Failed to marshal response"
			logger.Error(msg, zap.Error(err))
			writeJSONError(logger, w, msg)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(resp)
		if err != nil {
			logger.Error("Error writing response", zap.Error(err))
		}
	})
}
