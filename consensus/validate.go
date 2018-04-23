package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func (c *Consensus) validateBlockHeader(header *block.Header, parent *block.Header, nowTimestamp uint64) error {

	if header.Timestamp() <= parent.Timestamp() {
		return errors.New("block timestamp too small")
	}
	if (header.Timestamp()-parent.Timestamp())%thor.BlockInterval != 0 {
		return errors.New("invalid block interval")
	}

	if header.Timestamp() > nowTimestamp+thor.BlockInterval {
		return errFutureBlock
	}

	if !block.GasLimit(header.GasLimit()).IsValid(parent.GasLimit()) {
		return errors.New("invalid block gas limit")
	}

	if header.GasUsed() > header.GasLimit() {
		return errors.New("block gas used exceeds limit")
	}

	if header.TotalScore() <= parent.TotalScore() {
		return errors.New("block total score too small")
	}
	return nil
}

func (c *Consensus) validateProposer(header *block.Header, parent *block.Header, st *state.State) error {
	signer, err := header.Signer()
	if err != nil {
		return errors.Wrap(err, "invalid block signer")
	}

	authority := builtin.Authority.Native(st)
	endorsement := builtin.Params.Native(st).Get(thor.KeyProposerEndorsement)

	candidates := authority.Candidates()
	proposers := make([]poa.Proposer, 0, len(candidates))
	for _, c := range candidates {
		if st.GetBalance(c.Endorsor).Cmp(endorsement) >= 0 {
			proposers = append(proposers, poa.Proposer{
				Address: c.Signer,
				Active:  c.Active,
			})
		}
	}

	sched, err := poa.NewScheduler(signer, proposers, parent.Number(), parent.Timestamp())
	if err != nil {
		return err
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return errors.New("block timestamp not scheduled")
	}

	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return errors.New("incorrect block total score")
	}

	for _, proposer := range updates {
		authority.Update(proposer.Address, proposer.Active)
	}
	return nil
}

func (c *Consensus) validateBlockBody(blk *block.Block) error {
	header := blk.Header()
	txs := blk.Transactions()
	if header.TxsRoot() != txs.RootHash() {
		return errors.New("incorrect block txs root")
	}

	for _, tx := range txs {
		switch {
		case tx.ChainTag() != c.chain.Tag():
			return errors.New("bad tx: chain tag mismatch")
		case header.Number() < tx.BlockRef().Number():
			return errors.New("bad tx: ref blcok too new")
		case tx.IsExpired(header.Number()):
			return errors.New("bad tx: expired")
		case tx.HasReservedFields():
			return errors.New("bad tx: reserved fields not empty")
		}
	}

	return nil
}
