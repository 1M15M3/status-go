package e2e

import (
	"time"

	"github.com/ethereum/go-ethereum/les"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/status-im/status-go/geth/api"
	"github.com/status-im/status-go/geth/common"
	"github.com/status-im/status-go/geth/signal"
	"github.com/stretchr/testify/suite"
)

// NodeManagerTestSuite defines a test suit with NodeManager.
type NodeManagerTestSuite struct {
	suite.Suite
	NodeManager common.NodeManager
}

// StartTestNode initiazes a NodeManager instances with configuration retrieved
// from the test config.
func (s *NodeManagerTestSuite) StartTestNode(networkID int, opts ...TestNodeOption) {
	nodeConfig, err := MakeTestNodeConfig(networkID)
	s.NoError(err)

	// Apply any options altering node config.
	for i := range opts {
		opts[i](nodeConfig)
	}

	// import account keys
	s.NoError(importTestAccouns(nodeConfig.KeyStoreDir))

	s.False(s.NodeManager.IsNodeRunning())
	nodeStarted, err := s.NodeManager.StartNode(nodeConfig)
	s.NoError(err)
	s.NotNil(nodeStarted)
	<-nodeStarted
	s.True(s.NodeManager.IsNodeRunning())
}

// StopTestNode attempts to stop initialized NodeManager.
func (s *NodeManagerTestSuite) StopTestNode() {
	s.NotNil(s.NodeManager)
	s.True(s.NodeManager.IsNodeRunning())
	nodeStopped, err := s.NodeManager.StopNode()
	s.NoError(err)
	<-nodeStopped
	s.False(s.NodeManager.IsNodeRunning())
}

// BackendTestSuite is a test suite with api.StatusBackend initialized
// and a few utility methods to start and stop node or get various services.
type BackendTestSuite struct {
	suite.Suite
	Backend *api.StatusBackend
}

// SetupTest initializes Backend.
func (s *BackendTestSuite) SetupTest() {
	s.Backend = api.NewStatusBackend()
	s.NotNil(s.Backend)
}

// TearDownTest cleans up the packages state.
func (s *BackendTestSuite) TearDownTest() {
	signal.ResetDefaultNodeNotificationHandler()
}

// StartTestBackend imports some keys and starts a node.
func (s *BackendTestSuite) StartTestBackend(networkID int, opts ...TestNodeOption) {
	nodeConfig, err := MakeTestNodeConfig(networkID)
	s.NoError(err)

	// Apply any options altering node config.
	for i := range opts {
		opts[i](nodeConfig)
	}

	// import account keys
	s.NoError(importTestAccouns(nodeConfig.KeyStoreDir))

	// start node
	s.False(s.Backend.IsNodeRunning())
	nodeStarted, err := s.Backend.StartNode(nodeConfig)
	s.NoError(err)
	<-nodeStarted
	s.True(s.Backend.IsNodeRunning())
}

// StopTestBackend stops the node.
func (s *BackendTestSuite) StopTestBackend() {
	s.True(s.Backend.IsNodeRunning())
	backendStopped, err := s.Backend.StopNode()
	s.NoError(err)
	<-backendStopped
	s.False(s.Backend.IsNodeRunning())
}

// RestartTestNode restarts a currently running node.
func (s *BackendTestSuite) RestartTestNode() {
	s.True(s.Backend.IsNodeRunning())
	nodeRestarted, err := s.Backend.RestartNode()
	s.NoError(err)
	<-nodeRestarted
	s.True(s.Backend.IsNodeRunning())
}

// EnsureSynchronization makes a test waiting until the LES
// is synced. It solves "no suitable peers available" issues
// happen when trying to ask for blockchain information too
// early.
// TODO(themue) Integrate into the tests.
func (s *BackendTestSuite) EnsureSynchronization() {
	start := time.Now()
	les, err := s.Backend.NodeManager().LightEthereumService()
	if err != nil {
		s.Error(err)
	}

	// Make sure LES finished synchronization. Actually,
	// this solves "no suitable peers" issue.
	// This issue appears only when we try to ask for blockchain information
	// before LES is synced.
	for {
		isSyncing := les.Downloader().Synchronising()
		progress := les.Downloader().Progress()

		if !isSyncing && progress.HighestBlock > 0 && progress.CurrentBlock >= progress.HighestBlock {
			break
		}

		time.Sleep(time.Second * 10)
		s.True(time.Now().Sub(start) < (5 * time.Minute))
	}
}

// WhisperService returns a reference to the Whisper service.
func (s *BackendTestSuite) WhisperService() *whisper.Whisper {
	whisperService, err := s.Backend.NodeManager().WhisperService()
	s.NoError(err)
	s.NotNil(whisperService)

	return whisperService
}

// LightEthereumService returns a reference to the LES service.
func (s *BackendTestSuite) LightEthereumService() *les.LightEthereum {
	lightEthereum, err := s.Backend.NodeManager().LightEthereumService()
	s.NoError(err)
	s.NotNil(lightEthereum)

	return lightEthereum
}

// TxQueueManager returns a reference to the TxQueueManager.
func (s *BackendTestSuite) TxQueueManager() common.TxQueueManager {
	return s.Backend.TxQueueManager()
}

func importTestAccouns(keyStoreDir string) (err error) {
	err = common.ImportTestAccount(keyStoreDir, "test-account1.pk")
	if err != nil {
		return
	}

	return common.ImportTestAccount(keyStoreDir, "test-account2.pk")
}
