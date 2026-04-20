package auth

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// MetadataKeyUserID は「認証済みユーザー ID」を運ぶ gRPC metadata キー。
// 値を入れるのは infra 側 Envoy (JWT 検証後に claims.sub を x-user-id として注入)。
// app サービスは読む側。
const MetadataKeyUserID = "x-user-id"

// RequesterID は incoming metadata の x-user-id を取り出す。
// gRPC ハンドラで「呼び出し元ユーザーは誰か」を判定したい時に使う。
//
// 使用例:
//   - UpdateUser: 呼び出し元 == 変更対象か (= 他人のプロフィールを書き換えさせない)
//   - CreateRoom: 作成者 ID として採用
//   - JoinRoom / LeaveRoom: 誰が Join/Leave するか
//   - ListMyRooms: 誰の参加ルーム一覧か
//
// JWT の検証はしない (Envoy の責務)。metadata が無い / 空なら ok=false を返すので、
// 呼び出し側は認証前提のエンドポイントでは Unauthenticated 等にマップすること。
func RequesterID(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	ids := md.Get(MetadataKeyUserID)
	if len(ids) == 0 || ids[0] == "" {
		return "", false
	}
	return ids[0], true
}

// PropagateRequester は incoming の x-user-id を outgoing context にコピーする。
// app サービス間で gRPC 呼び出しをする時に使う — Envoy は外側にしか噛んでいないので、
// このコピーをしないと下流サービスから「呼び出し元が誰か」が見えなくなる。
//
// 使用例:
//   - chat-service が user-service の GetUser を呼んでメンバー情報を enrich する時
//     (userclient.Client.GetUser の内部で呼ばれる)
//
// 呼び出し元ユーザーが不明 (= x-user-id が無い) 場合は何もせず ctx をそのまま返す。
func PropagateRequester(ctx context.Context) context.Context {
	id, ok := RequesterID(ctx)
	if !ok {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataKeyUserID, id)
}
