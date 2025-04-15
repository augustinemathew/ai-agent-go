package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestCollectAndConcatenateResults(t *testing.T) {
	testCases := []struct {
		name           string
		inputMessages  []OutputResult
		expectedResult OutputResult // Expect OutputResult now
	}{
		{
			name:           "No Messages",
			inputMessages:  []OutputResult{},
			expectedResult: OutputResult{}, // Expect zero OutputResult
		},
		{
			name: "Single Success Message",
			inputMessages: []OutputResult{
				{TaskID: "single-1", Status: StatusSucceeded, Message: "Done"},
			},
			expectedResult: OutputResult{ // Data is empty, fields match the single input
				TaskID:     "single-1",
				Status:     StatusSucceeded,
				Message:    "Done",
				ResultData: "",
			},
		},
		{
			name: "Single Failure Message",
			inputMessages: []OutputResult{
				{TaskID: "single-fail-1", Status: StatusFailed, Message: "Oops", Error: "exit 1"},
			},
			expectedResult: OutputResult{ // Data is empty, fields match the single input
				TaskID:     "single-fail-1",
				Status:     StatusFailed,
				Message:    "Oops",
				Error:      "exit 1",
				ResultData: "",
			},
		},
		{
			name: "Streaming Success (e.g., FileRead)",
			inputMessages: []OutputResult{
				{TaskID: "stream-1", Status: StatusRunning, Message: "Reading...", ResultData: "Chunk 1 data. "},
				{TaskID: "stream-1", Status: StatusRunning, Message: "Still reading...", ResultData: "Chunk 2 data!"},
				{TaskID: "stream-1", Status: StatusSucceeded, Message: "Finished reading."},
			},
			expectedResult: OutputResult{ // Fields from last message, data concatenated
				TaskID:     "stream-1",
				Status:     StatusSucceeded,
				Message:    "Finished reading.",
				ResultData: "Chunk 1 data. Chunk 2 data!",
			},
		},
		{
			name: "Streaming Failure (e.g., BashExec)",
			inputMessages: []OutputResult{
				{TaskID: "stream-fail-1", Status: StatusRunning, Message: "Running...", ResultData: "stdout line 1\n"},
				{TaskID: "stream-fail-1", Status: StatusRunning, Message: "Still running...", ResultData: "stderr line 1\n"},
				{TaskID: "stream-fail-1", Status: StatusFailed, Message: "Command failed.", Error: "exit code 127"},
			},
			expectedResult: OutputResult{ // Fields from last message, data concatenated
				TaskID:     "stream-fail-1",
				Status:     StatusFailed,
				Message:    "Command failed.",
				Error:      "exit code 127",
				ResultData: "stdout line 1\nstderr line 1\n",
			},
		},
		{
			name: "Streaming with Empty Data Chunks",
			inputMessages: []OutputResult{
				{TaskID: "stream-empty-1", Status: StatusRunning, Message: "Reading...", ResultData: "Data Chunk A."}, // Has data
				{TaskID: "stream-empty-1", Status: StatusRunning, Message: "Reading..."},                              // No data
				{TaskID: "stream-empty-1", Status: StatusRunning, Message: "Reading...", ResultData: "Data Chunk B."}, // Has data
				{TaskID: "stream-empty-1", Status: StatusSucceeded, Message: "Done."},
			},
			expectedResult: OutputResult{ // Fields from last message, data concatenated (empty ignored)
				TaskID:     "stream-empty-1",
				Status:     StatusSucceeded,
				Message:    "Done.",
				ResultData: "Data Chunk A.Data Chunk B.",
			},
		},
		{
			name: "Large File Simulation (500 chunks)",
			inputMessages: func() []OutputResult {
				chunkCount := 500
				msgs := make([]OutputResult, 0, chunkCount+1)
				for i := 0; i < chunkCount; i++ {
					msgs = append(msgs, OutputResult{
						TaskID:     "large-file-1",
						Status:     StatusRunning,
						ResultData: fmt.Sprintf("data_chunk_%d;", i),
					})
				}
				msgs = append(msgs, OutputResult{TaskID: "large-file-1", Status: StatusSucceeded, Message: "Large read done."})
				return msgs
			}(),
			expectedResult: OutputResult{ // Fields from last message, data concatenated
				TaskID:  "large-file-1",
				Status:  StatusSucceeded,
				Message: "Large read done.",
				ResultData: func() string {
					var sb strings.Builder
					for i := 0; i < 500; i++ {
						sb.WriteString(fmt.Sprintf("data_chunk_%d;", i))
					}
					return sb.String()
				}(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create channel and send messages in a goroutine
			resultsChan := make(chan OutputResult, len(tc.inputMessages)+1) // Buffer slightly larger
			go func() {
				defer close(resultsChan) // IMPORTANT: Close channel when done sending
				for _, msg := range tc.inputMessages {
					resultsChan <- msg
				}
			}()

			// Act: Call the function under test
			actualResult := CombineOutputResults(context.Background(), resultsChan)

			// Assert: Compare the actual result with the expected result
			if diff := cmp.Diff(tc.expectedResult, actualResult); diff != "" {
				t.Errorf("CollectAndConcatenateResults mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCombineOutputResults(t *testing.T) {
	// --- Test Cases for Normal Completion ---
	normalTestCases := []struct {
		name           string
		inputMessages  []OutputResult
		expectedResult OutputResult
	}{
		{
			name:           "Normal - No Messages",
			inputMessages:  []OutputResult{},
			expectedResult: OutputResult{}, // Expect zero OutputResult
		},
		{
			name: "Normal - Single Success Message",
			inputMessages: []OutputResult{
				{TaskID: "single-1", Status: StatusSucceeded, Message: "Done"},
			},
			expectedResult: OutputResult{
				TaskID:     "single-1",
				Status:     StatusSucceeded,
				Message:    "Done",
				ResultData: "",
			},
		},
		{
			name: "Normal - Single Failure Message",
			inputMessages: []OutputResult{
				{TaskID: "single-fail-1", Status: StatusFailed, Message: "Oops", Error: "exit 1"},
			},
			expectedResult: OutputResult{
				TaskID:     "single-fail-1",
				Status:     StatusFailed,
				Message:    "Oops",
				Error:      "exit 1",
				ResultData: "",
			},
		},
		{
			name: "Normal - Streaming Success",
			inputMessages: []OutputResult{
				{TaskID: "stream-1", Status: StatusRunning, Message: "Reading...", ResultData: "Chunk 1 data. "},
				{TaskID: "stream-1", Status: StatusRunning, Message: "Still reading...", ResultData: "Chunk 2 data!"},
				{TaskID: "stream-1", Status: StatusSucceeded, Message: "Finished reading."},
			},
			expectedResult: OutputResult{
				TaskID:     "stream-1",
				Status:     StatusSucceeded,
				Message:    "Finished reading.",
				ResultData: "Chunk 1 data. Chunk 2 data!",
			},
		},
		{
			name: "Normal - Streaming Failure",
			inputMessages: []OutputResult{
				{TaskID: "stream-fail-1", Status: StatusRunning, Message: "Running...", ResultData: "stdout line 1\n"},
				{TaskID: "stream-fail-1", Status: StatusRunning, Message: "Still running...", ResultData: "stderr line 1\n"},
				{TaskID: "stream-fail-1", Status: StatusFailed, Message: "Command failed.", Error: "exit code 127"},
			},
			expectedResult: OutputResult{
				TaskID:     "stream-fail-1",
				Status:     StatusFailed,
				Message:    "Command failed.",
				Error:      "exit code 127",
				ResultData: "stdout line 1\nstderr line 1\n",
			},
		},
		{
			name: "Normal - Large File Simulation (500 chunks)",
			inputMessages: func() []OutputResult {
				chunkCount := 500
				msgs := make([]OutputResult, 0, chunkCount+1)
				for i := 0; i < chunkCount; i++ {
					msgs = append(msgs, OutputResult{
						TaskID:     "large-file-1",
						Status:     StatusRunning,
						ResultData: fmt.Sprintf("data_chunk_%d;", i),
					})
				}
				msgs = append(msgs, OutputResult{TaskID: "large-file-1", Status: StatusSucceeded, Message: "Large read done."})
				return msgs
			}(),
			expectedResult: OutputResult{
				TaskID:  "large-file-1",
				Status:  StatusSucceeded,
				Message: "Large read done.",
				ResultData: func() string {
					var sb strings.Builder
					for i := 0; i < 500; i++ {
						sb.WriteString(fmt.Sprintf("data_chunk_%d;", i))
					}
					return sb.String()
				}(),
			},
		},
	}

	for _, tc := range normalTestCases {
		t.Run(tc.name, func(t *testing.T) {
			resultsChan := make(chan OutputResult, len(tc.inputMessages)+1)
			go func() {
				defer close(resultsChan)
				for _, msg := range tc.inputMessages {
					resultsChan <- msg
				}
			}()

			actualResult := CombineOutputResults(context.Background(), resultsChan) // Use background context

			if diff := cmp.Diff(tc.expectedResult, actualResult); diff != "" {
				t.Errorf("CombineOutputResults mismatch (-want +got):\n%s", diff)
			}
		})
	}

	// --- Test Cases for Context Cancellation ---
	t.Run("Cancellation - Before Any Message", func(t *testing.T) {
		resultsChan := make(chan OutputResult, 1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		actualResult := CombineOutputResults(ctx, resultsChan)

		expectedResult := OutputResult{
			Status:     StatusFailed,
			Error:      context.Canceled.Error(),
			Message:    "Result collection cancelled for command .", // CommandID is empty
			ResultData: "",
		}
		if diff := cmp.Diff(expectedResult, actualResult); diff != "" {
			t.Errorf("CombineOutputResults mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Cancellation - After Some Messages", func(t *testing.T) {
		resultsChan := make(chan OutputResult, 5)
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			// Send a few messages then cancel
			resultsChan <- OutputResult{TaskID: "cancel-mid-1", Status: StatusRunning, ResultData: "Part 1."}
			time.Sleep(10 * time.Millisecond) // Give collector a chance to read
			resultsChan <- OutputResult{TaskID: "cancel-mid-1", Status: StatusRunning, ResultData: "Part 2."}
			time.Sleep(10 * time.Millisecond)
			cancel() // Cancel the context
			// Don't close the channel, let cancellation handle termination
		}()

		actualResult := CombineOutputResults(ctx, resultsChan)
		wg.Wait() // Ensure goroutine finishes before assertion

		expectedResult := OutputResult{
			TaskID:     "cancel-mid-1", // Should have ID from last message read
			Status:     StatusFailed,
			Error:      context.Canceled.Error(),
			Message:    "Result collection cancelled for command cancel-mid-1.",
			ResultData: "Part 1.Part 2.", // Data collected before cancel
		}
		if diff := cmp.Diff(expectedResult, actualResult); diff != "" {
			t.Errorf("CombineOutputResults mismatch (-want +got):\n%s", diff)
		}
	})

}
