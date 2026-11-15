// 消息悬浮窗的状态（反馈 #79）：消息不再是独立页面，而是浮在当前页面之上的窗口。
// 打开时可以直接定位到与某人的会话；旧的 /messages 与 /messages/<用户名> 地址打开同一个窗口（见 pages/MessagesRedirect）。
class Chat {
  open = $state(false);
  /** 当前会话的对方用户名；空表示显示会话列表 */
  peer = $state("");

  show(peer = ""): void {
    this.peer = peer;
    this.open = true;
  }

  toggle(): void {
    if (this.open) this.close();
    else this.show(this.peer);
  }

  close(): void {
    this.open = false;
  }
}

export const chat = new Chat();
