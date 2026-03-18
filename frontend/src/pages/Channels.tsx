import { useState, useEffect } from "react";
import { Trash2, Send, Slack, SendHorizontal, MessageSquare, Save, Plus, X, Inbox, Share2, AlertCircle, CheckCircle2 } from "lucide-react";

const Channels = () => {
  const [credentials, setCredentials] = useState<any[]>([]);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<"input" | "output">("input");
  const [activePlatform, setActivePlatform] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Form states
  const [channelName, setChannelName] = useState("");
  const [slackToken, setSlackToken] = useState("");
  const [dingtalkWebhook, setDingtalkWebhook] = useState("");

  // Telegram Auth Flow
  const [telegramPhone, setTelegramPhone] = useState("");
  const [telegramCode, setTelegramCode] = useState("");
  const [telegramStep, setTelegramStep] = useState<"phone" | "code">("phone");

  const inputPlatforms = [
    { id: "slack", label: "Slack", icon: Slack, color: "text-purple-600", bg: "bg-purple-50" },
    { id: "telegram", label: "Telegram", icon: SendHorizontal, color: "text-sky-500", bg: "bg-sky-50" },
    { id: "lark", label: "Lark", icon: MessageSquare, color: "text-emerald-500", bg: "bg-emerald-50" },
  ];

  const outputPlatforms = [
    { id: "dingtalk", label: "DingTalk", icon: Send, color: "text-blue-500", bg: "bg-blue-50" },
  ];

  const fetchCredentials = async () => {
    try {
      const response = await fetch("/api/credentials", {
        headers: { "Authorization": `Bearer ${localStorage.getItem("token")}` }
      });
      if (response.ok) setCredentials(await response.json());
    } catch (err) {
      console.error("Failed to fetch credentials:", err);
    }
  };

  const completeLarkAuth = async (code: string, name: string) => {
    setIsLoading(true);
    try {
      const response = await fetch("/api/lark/auth/callback", {
        method: "POST",
        headers: { 
          "Content-Type": "application/json", 
          "Authorization": `Bearer ${localStorage.getItem("token")}` 
        },
        body: JSON.stringify({ code, name }),
      });
      if (!response.ok) throw new Error("Failed to connect Lark");
      setMessage({ type: 'success', text: "Lark connected successfully!" });
      fetchCredentials();
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleLarkAuth = async () => {
    if (!channelName) {
      setMessage({ type: 'error', text: "Please enter a display name first" });
      return;
    }
    setIsLoading(true);
    try {
      // Save name to localStorage to recover after redirect
      localStorage.setItem('lark_pending_name', channelName);
      
      const redirectUri = window.location.origin + window.location.pathname;
      const response = await fetch(`/api/lark/auth/url?redirect_uri=${encodeURIComponent(redirectUri)}`, {
        headers: { "Authorization": `Bearer ${localStorage.getItem("token")}` }
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Failed to get Lark auth URL");
      
      // Redirect to Lark
      window.location.href = data.url;
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchCredentials();

    // Handle Lark OAuth Callback
    const urlParams = new URLSearchParams(window.location.search);
    const code = urlParams.get('code');
    const state = urlParams.get('state');
    
    if (code && state === 'lark-auth') {
      const savedName = localStorage.getItem('lark_pending_name');
      if (savedName) {
        completeLarkAuth(code, savedName);
        // Clear URL and storage
        window.history.replaceState({}, document.title, window.location.pathname);
        localStorage.removeItem('lark_pending_name');
      }
    }
  }, []);

  const openDrawer = () => {
    setChannelName("");
    setTelegramPhone("");
    setTelegramCode("");
    setTelegramStep("phone");
    setActivePlatform(activeTab === "input" ? "slack" : "dingtalk");
    setIsDrawerOpen(true);
    setMessage(null);
  };

  const handleTelegramSendCode = async () => {
    setIsLoading(true);
    setMessage(null);
    try {
      const response = await fetch("/api/telegram/auth/send-code", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": `Bearer ${localStorage.getItem("token")}` },
        body: JSON.stringify({ phone: telegramPhone }),
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Failed to send code");
      setTelegramStep("code");
      setMessage({ type: 'success', text: "Verification code sent to your Telegram!" });
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleTelegramVerifyCode = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!channelName) {
      setMessage({ type: 'error', text: "Please enter a display name first" });
      return;
    }
    setIsLoading(true);
    setMessage(null);
    try {
      const response = await fetch("/api/telegram/auth/verify-code", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": `Bearer ${localStorage.getItem("token")}` },
        body: JSON.stringify({ code: telegramCode, name: channelName }),
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Failed to verify code");
      setMessage({ type: 'success', text: "Telegram connected successfully!" });
      fetchCredentials();
      setTimeout(() => setIsDrawerOpen(false), 1500);
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (activePlatform === "telegram") {
      if (telegramStep === "phone") handleTelegramSendCode();
      else handleTelegramVerifyCode(e);
      return;
    }

    setIsLoading(true);
    let tokenValue = "";
    if (activePlatform === "slack") tokenValue = slackToken;
    else if (activePlatform === "dingtalk") tokenValue = dingtalkWebhook;

    try {
      const response = await fetch("/api/credentials", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": `Bearer ${localStorage.getItem("token")}` },
        body: JSON.stringify({ name: channelName, source_type: activePlatform, token: tokenValue }),
      });
      if (!response.ok) throw new Error("Failed to save channel");
      setMessage({ type: 'success', text: "Channel saved!" });
      fetchCredentials();
      setTimeout(() => setIsDrawerOpen(false), 1500);
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm("Delete this channel?")) return;
    await fetch(`/api/credentials/${id}`, {
      method: "DELETE",
      headers: { "Authorization": `Bearer ${localStorage.getItem("token")}` }
    });
    fetchCredentials();
  };

  const filteredList = credentials.filter(c => 
    activeTab === "input" ? ["slack", "telegram", "lark"].includes(c.source_type) : c.source_type === "dingtalk"
  );

  return (
    <div className="space-y-8 animate-in slide-in-from-bottom-4 duration-500">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight text-slate-900">IM Channels</h2>
          <p className="text-slate-500 mt-1">Manage your message sources and destinations.</p>
        </div>
        <button 
          onClick={openDrawer}
          className="bg-blue-600 hover:bg-blue-700 text-white px-6 py-3 rounded-2xl shadow-lg shadow-blue-200 flex items-center gap-2 font-bold transition-all active:scale-[0.98]"
        >
          <Plus className="w-5 h-5" />
          Create {activeTab}
        </button>
      </header>

      {/* Tabs */}
      <div className="flex p-1 bg-slate-200/50 rounded-2xl w-fit">
        <button
          onClick={() => setActiveTab("input")}
          className={`flex items-center gap-2 px-8 py-3 rounded-xl text-sm font-black uppercase tracking-wider transition-all ${
            activeTab === "input" ? "bg-white text-blue-600 shadow-sm" : "text-slate-500 hover:text-slate-700"
          }`}
        >
          <Inbox className="w-4 h-4" />
          Inputs
        </button>
        <button
          onClick={() => setActiveTab("output")}
          className={`flex items-center gap-2 px-8 py-3 rounded-xl text-sm font-black uppercase tracking-wider transition-all ${
            activeTab === "output" ? "bg-white text-blue-600 shadow-sm" : "text-slate-500 hover:text-slate-700"
          }`}
        >
          <Share2 className="w-4 h-4" />
          Outputs
        </button>
      </div>

      {/* List Card */}
      <div className="bg-white p-8 rounded-[40px] border border-slate-100 shadow-sm min-h-[400px]">
        <div className="space-y-4">
          {filteredList.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-center">
              <div className="bg-slate-50 p-6 rounded-[32px] mb-4">
                {activeTab === "input" ? <Inbox className="w-12 h-12 text-slate-200" /> : <Share2 className="w-12 h-12 text-slate-200" />}
              </div>
              <h3 className="text-lg font-bold text-slate-900">No {activeTab} channels found</h3>
              <p className="text-slate-400 max-w-xs mt-1">Click the create button above to add your first {activeTab} platform.</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {filteredList.map(c => (
                <div key={c.id} className="p-6 bg-slate-50/50 rounded-[32px] border border-slate-100 group hover:bg-white hover:shadow-md hover:border-blue-100 transition-all duration-300">
                  <div className="flex items-start justify-between mb-4">
                    <div className="bg-white p-3 rounded-2xl shadow-sm ring-1 ring-slate-100">
                      {c.source_type === 'slack' && <Slack className="w-6 h-6 text-purple-600" />}
                      {c.source_type === 'telegram' && <SendHorizontal className="w-6 h-6 text-sky-500" />}
                      {c.source_type === 'lark' && <MessageSquare className="w-6 h-6 text-emerald-500" />}
                      {c.source_type === 'dingtalk' && <Send className="w-6 h-6 text-blue-500" />}
                    </div>
                    <button onClick={() => handleDelete(c.id)} className="p-2 text-slate-300 hover:text-rose-500 hover:bg-rose-50 rounded-xl transition-all opacity-0 group-hover:opacity-100">
                      <Trash2 className="w-5 h-5" />
                    </button>
                  </div>
                  <div>
                    <p className="font-black text-slate-900 text-lg leading-tight">{c.name}</p>
                    <p className="text-[10px] text-slate-400 font-black uppercase tracking-[0.2em] mt-1">{c.source_type} / ID: {c.id}</p>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Drawer Overlay */}
      {isDrawerOpen && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div className="absolute inset-0 bg-slate-900/40 backdrop-blur-sm animate-in fade-in duration-300" onClick={() => setIsDrawerOpen(false)} />
          
          <div className="relative w-full max-w-md bg-white h-full shadow-2xl animate-in slide-in-from-right duration-300 flex flex-col">
            <div className="p-8 border-b border-slate-100 flex items-center justify-between bg-slate-50/50">
              <div>
                <h3 className="text-2xl font-black tracking-tight text-slate-900">Configure {activeTab}</h3>
                <p className="text-sm text-slate-500 mt-1 font-medium">Link a new platform to your account</p>
              </div>
              <button onClick={() => setIsDrawerOpen(false)} className="p-2 hover:bg-white rounded-2xl transition-colors text-slate-400 hover:text-slate-900">
                <X className="w-6 h-6" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto p-8 space-y-10">
              {message && (
                <div className={`p-4 rounded-2xl flex items-center gap-3 animate-in zoom-in duration-300 ${
                  message.type === 'success' ? 'bg-emerald-50 text-emerald-700' : 'bg-rose-50 text-rose-700'
                }`}>
                  {message.type === 'success' ? <CheckCircle2 className="w-5 h-5" /> : <AlertCircle className="w-5 h-5" />}
                  <p className="text-sm font-bold">{message.text}</p>
                </div>
              )}

              <div className="space-y-4">
                <label className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400 ml-1">1. Choose Platform</label>
                <div className="grid grid-cols-2 gap-3">
                  {(activeTab === "input" ? inputPlatforms : outputPlatforms).map(p => (
                    <button
                      key={p.id}
                      onClick={() => setActivePlatform(p.id)}
                      className={`flex items-center gap-3 p-4 rounded-3xl border-2 transition-all duration-200 ${
                        activePlatform === p.id ? "border-blue-600 bg-blue-50/50 text-blue-700 shadow-sm" : "border-slate-100 hover:border-slate-200 text-slate-600"
                      }`}
                    >
                      <p.icon className={`w-5 h-5 ${activePlatform === p.id ? "text-blue-600" : "text-slate-400"}`} />
                      <span className="font-bold text-sm">{p.label}</span>
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-6">
                <label className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400 ml-1">2. Basic Info</label>
                <div className="space-y-2">
                  <label className="text-sm font-bold text-slate-700 ml-1">Display Name</label>
                  <input 
                    type="text" 
                    value={channelName} 
                    onChange={e => setChannelName(e.target.value)} 
                    className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 focus:border-blue-500 outline-none transition-all placeholder:text-slate-300" 
                    placeholder="e.g. My Personal Slack" 
                    required 
                  />
                </div>

                <label className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400 ml-1 block pt-4">3. Credentials</label>
                <form onSubmit={handleSave} className="space-y-6">
                  {activePlatform === "slack" && (
                    <div className="space-y-2">
                      <label className="text-sm font-bold text-slate-700 ml-1">User OAuth Token</label>
                      <input type="password" value={slackToken} onChange={e => setSlackToken(e.target.value)} className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 focus:border-blue-500 outline-none transition-all placeholder:text-slate-300" placeholder="xoxp-..." required />
                    </div>
                  )}
                  {activePlatform === "telegram" && (
                    <div className="space-y-5 animate-in fade-in slide-in-from-bottom-2 duration-300">
                      {telegramStep === "phone" ? (
                        <div className="space-y-2">
                          <label className="text-sm font-bold text-slate-700 ml-1">Phone Number</label>
                          <input 
                            type="tel" 
                            value={telegramPhone} 
                            onChange={e => setTelegramPhone(e.target.value)} 
                            className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 outline-none" 
                            placeholder="+1234567890" 
                            required 
                          />
                          <p className="text-[11px] text-slate-400 ml-1">Include country code, e.g. +1 for USA</p>
                        </div>
                      ) : (
                        <div className="space-y-2">
                          <div className="flex items-center justify-between ml-1">
                            <label className="text-sm font-bold text-slate-700">Verification Code</label>
                            <button 
                              type="button" 
                              onClick={() => setTelegramStep("phone")}
                              className="text-xs text-blue-600 font-bold hover:underline"
                            >
                              Change Phone
                            </button>
                          </div>
                          <input 
                            type="text" 
                            value={telegramCode} 
                            onChange={e => setTelegramCode(e.target.value)} 
                            className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 outline-none text-center text-2xl font-black tracking-[0.5em]" 
                            placeholder="00000" 
                            maxLength={5}
                            required 
                          />
                        </div>
                      )}
                    </div>
                  )}
                  {activePlatform === "lark" && (
                    <div className="space-y-4 animate-in fade-in slide-in-from-bottom-2 duration-300">
                      <div className="p-6 bg-emerald-50 rounded-[32px] border border-emerald-100 flex flex-col items-center text-center gap-4">
                        <div className="bg-white p-4 rounded-2xl shadow-sm">
                          <MessageSquare className="w-10 h-10 text-emerald-500" />
                        </div>
                        <div>
                          <p className="font-bold text-emerald-900">Connect Lark Account</p>
                          <p className="text-xs text-emerald-600 mt-1">You will be redirected to Lark to authorize this application.</p>
                        </div>
                        <button
                          type="button"
                          onClick={handleLarkAuth}
                          disabled={isLoading}
                          className="w-full bg-emerald-600 hover:bg-emerald-700 text-white font-black py-4 rounded-2xl shadow-lg shadow-emerald-100 flex items-center justify-center gap-2 transition-all active:scale-[0.98]"
                        >
                          <Share2 className="w-5 h-5" />
                          Authorize via Lark
                        </button>
                      </div>
                    </div>
                  )}
                  {activePlatform === "dingtalk" && (
                    <div className="space-y-2">
                      <label className="text-sm font-bold text-slate-700 ml-1">Webhook URL</label>
                      <input type="url" value={dingtalkWebhook} onChange={e => setDingtalkWebhook(e.target.value)} className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 focus:border-blue-500 outline-none" placeholder="https://..." required />
                    </div>
                  )}

                  <button
                    type="submit"
                    disabled={isLoading}
                    className={`w-full text-white font-black py-5 rounded-[32px] shadow-2xl flex items-center justify-center gap-3 active:scale-[0.98] transition-all disabled:opacity-70 mt-4 ${
                      activePlatform === 'lark' ? 'hidden' : 'bg-slate-900'
                    }`}
                  >
                    {isLoading ? (
                      <div className="w-6 h-6 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    ) : (
                      <Save className="w-6 h-6" />
                    )}
                    {activePlatform === "telegram" 
                      ? (telegramStep === "phone" ? "Send Verification Code" : "Verify & Save Telegram")
                      : `Save ${activePlatform.toUpperCase()}`
                    }
                  </button>
                </form>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default Channels;
