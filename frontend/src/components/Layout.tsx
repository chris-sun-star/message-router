import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { MessageSquareText, Layers, Radio, Bot, LogOut, User, ChevronDown } from "lucide-react";
import { cn } from "../lib/utils";
import { useState, useRef, useEffect } from "react";

const Layout = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [isUserMenuOpen, setIsUserMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const userName = localStorage.getItem("userName") || "User";

  const menuItems = [
    { icon: Layers, label: "Subscriptions", path: "/subscriptions" },
    { icon: Radio, label: "Channels", path: "/channels" },
    { icon: Bot, label: "AI Models", path: "/llms" },
  ];

  const handleLogout = () => {
    localStorage.removeItem("token");
    localStorage.removeItem("userName");
    navigate("/login");
  };

  // Close menu when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsUserMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  return (
    <div className="flex min-h-screen bg-slate-50 font-sans">
      {/* Sidebar */}
      <aside className="fixed left-0 top-0 h-full w-64 bg-white border-r border-slate-100 flex flex-col shadow-sm z-40">
        <div className="p-8">
          <div className="flex items-center gap-3 mb-10">
            <div className="bg-blue-600 p-2 rounded-xl shadow-lg shadow-blue-200">
              <MessageSquareText className="w-6 h-6 text-white" />
            </div>
            <span className="text-xl font-black text-slate-900 tracking-tight">MsgRouter</span>
          </div>

          <nav className="space-y-2">
            {menuItems.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                className={cn(
                  "flex items-center gap-3 px-4 py-3.5 rounded-2xl transition-all duration-200 group",
                  location.pathname === item.path
                    ? "bg-blue-50 text-blue-600 font-bold shadow-inner ring-1 ring-blue-100/50"
                    : "text-slate-500 hover:bg-slate-50 hover:text-slate-900"
                )}
              >
                <item.icon className={cn(
                  "w-5 h-5 transition-colors",
                  location.pathname === item.path ? "text-blue-600" : "text-slate-400 group-hover:text-slate-600"
                )} />
                {item.label}
              </Link>
            ))}
          </nav>
        </div>
        
        <div className="mt-auto p-8 text-center">
          <p className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-300">v1.0.0 Alpha</p>
        </div>
      </aside>

      {/* Main Content Area */}
      <div className="flex-1 ml-64 flex flex-col min-h-screen">
        {/* Top Header */}
        <header className="h-20 bg-white/80 backdrop-blur-md border-b border-slate-100 sticky top-0 z-30 px-8 flex items-center justify-end">
          <div className="relative" ref={menuRef}>
            <button 
              onClick={() => setIsUserMenuOpen(!isUserMenuOpen)}
              className="flex items-center gap-3 p-1.5 pl-4 hover:bg-slate-50 rounded-2xl transition-all border border-transparent hover:border-slate-100 group"
            >
              <div className="text-right hidden sm:block">
                <p className="text-sm font-black text-slate-900 leading-none">{userName}</p>
                <p className="text-[10px] font-bold text-blue-600 uppercase tracking-widest mt-1">Pro Account</p>
              </div>
              <div className="w-10 h-10 bg-blue-600 rounded-xl flex items-center justify-center text-white font-black shadow-lg shadow-blue-100">
                {userName.charAt(0).toUpperCase()}
              </div>
              <ChevronDown className={cn("w-4 h-4 text-slate-400 transition-transform duration-200", isUserMenuOpen && "rotate-180")} />
            </button>

            {/* Dropdown Menu */}
            {isUserMenuOpen && (
              <div className="absolute right-0 mt-2 w-56 bg-white rounded-[24px] shadow-2xl border border-slate-100 py-2 animate-in fade-in zoom-in-95 duration-200">
                <div className="px-4 py-3 border-b border-slate-50 mb-1">
                  <p className="text-xs font-black uppercase tracking-widest text-slate-400">Settings</p>
                </div>
                <button 
                  onClick={handleLogout}
                  className="w-full flex items-center gap-3 px-4 py-3 text-sm text-rose-600 hover:bg-rose-50 transition-colors font-black"
                >
                  <LogOut className="w-4 h-4" />
                  Sign Out
                </button>
              </div>
            )}
          </div>
        </header>

        {/* Page Content */}
        <main className="p-8 flex-1">
          <div className="max-w-6xl mx-auto">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
};

export default Layout;
