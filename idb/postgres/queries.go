package postgres


// Store metadata needed to rewind an account close transaction.
// the total_rewards field is reset when the account is closed so we need to attach the previous total to the txn.
const setTxnCloseExtraQuery = `
		WITH cramount AS
		(
		   SELECT
		   ($1)::bigint as round,
		   ($2)::bigint as intra,
		   x.rewards_total
		   FROM account x
		   WHERE x.addr = $3
		)
		UPDATE txn ut
		SET
			extra = jsonb_set
		(
		   coalesce(ut.extra, '{}'::jsonb),
		   '{aca}',
		   to_jsonb(cramount.rewards_total)
		)
		FROM cramount
		WHERE ut.round = cramount.round
		AND ut.intra = cramount.intra
`
