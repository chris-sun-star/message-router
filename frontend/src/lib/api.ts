export async function apiFetch(url: string, options: RequestInit = {}) {
  const token = localStorage.getItem("token");
  
  const headers = {
    ...options.headers,
    ...(token ? { "Authorization": `Bearer ${token}` } : {}),
  };

  const response = await fetch(url, { ...options, headers });

  if (response.status === 401) {
    // Token expired or invalid
    localStorage.removeItem("token");
    localStorage.removeItem("user");
    window.location.href = "/login";
    throw new Error("Session expired. Please login again.");
  }

  return response;
}
