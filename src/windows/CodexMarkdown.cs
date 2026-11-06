using System;
using System.Collections;
using System.Collections.Generic;
using System.Drawing;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        Application.Run(new ExportWindow(ExportWindow.DownloadsFolder));
    }
}

internal sealed class ExportWindow : Form
{
    private readonly TextBox search = new TextBox();
    private readonly ListBox titles = new ListBox();
    private readonly Label status = new Label();
    private readonly CodexHistory history = new CodexHistory();
    private readonly Func<string> downloadFolder;
    private List<Discussion> discussions = new List<Discussion>();
    private bool busy;

    internal ExportWindow(Func<string> downloadFolder)
    {
        this.downloadFolder = downloadFolder;
        Text = "Codex Markdown";
        StartPosition = FormStartPosition.CenterScreen;
        ClientSize = new Size(680, 510);
        MinimumSize = new Size(385, 270);
        AutoScaleMode = AutoScaleMode.Dpi;
        Font = new Font("Segoe UI", 10f);
        BackColor = Color.White;
        KeyPreview = true;

        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Fill, Padding = new Padding(20, 14, 20, 12),
            ColumnCount = 1, RowCount = 5
        };
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        for (var row = 0; row < 5; row++)
            layout.RowStyles.Add(new RowStyle(row == 3 ? SizeType.Percent : SizeType.AutoSize, 100));
        Controls.Add(layout);

        var heading = new Label
        {
            Text = "Discussions Codex", AutoSize = true, Margin = new Padding(0),
            Font = new Font(Font.FontFamily, 17f, FontStyle.Bold)
        };
        var help = new Label
        {
            Text = "Cliquez sur un titre pour l’exporter dans Téléchargements.",
            AutoSize = true, Margin = new Padding(0, 3, 0, 12), ForeColor = Color.DimGray
        };
        layout.Controls.Add(heading, 0, 0);
        layout.Controls.Add(help, 0, 1);

        search.AccessibleName = "Filtrer les discussions par titre";
        search.Dock = DockStyle.Top;
        search.Margin = new Padding(0, 0, 0, 9);
        search.HandleCreated += (sender, args) => SendMessage(search.Handle, 0x1501, IntPtr.Zero, "Filtrer par titre…");
        search.TextChanged += (sender, args) => ShowTitles();
        search.KeyDown += async (sender, args) =>
        {
            if (args.KeyCode != Keys.Enter || titles.Items.Count != 1) return;
            args.SuppressKeyPress = true;
            await ExportAsync((Discussion)titles.Items[0]);
        };
        layout.Controls.Add(search, 0, 2);

        titles.AccessibleName = "Discussions Codex. Appuyez sur Entrée pour exporter.";
        titles.Dock = DockStyle.Fill;
        titles.Margin = new Padding(0);
        titles.IntegralHeight = false;
        titles.HorizontalScrollbar = true;
        titles.MouseClick += async (sender, args) =>
        {
            if (args.Button != MouseButtons.Left) return;
            int index = titles.IndexFromPoint(args.Location);
            if (index >= 0 && titles.GetItemRectangle(index).Contains(args.Location))
                await ExportAsync((Discussion)titles.Items[index]);
        };
        titles.KeyDown += async (sender, args) =>
        {
            if (args.KeyCode != Keys.Enter || titles.SelectedItem == null) return;
            args.SuppressKeyPress = true;
            await ExportAsync((Discussion)titles.SelectedItem);
        };
        layout.Controls.Add(titles, 0, 3);

        status.AutoEllipsis = true;
        status.Dock = DockStyle.Top;
        status.Height = Font.Height + 4;
        status.Margin = new Padding(0, 9, 0, 0);
        status.ForeColor = Color.DimGray;
        layout.Controls.Add(status, 0, 4);

        Shown += async (sender, args) => await LoadAsync();
        KeyDown += async (sender, args) =>
        {
            if (args.KeyCode != Keys.F5) return;
            args.SuppressKeyPress = true;
            await LoadAsync();
        };
    }

    private async Task LoadAsync()
    {
        if (busy) return;
        busy = true;
        status.Text = "Chargement des discussions…";
        try
        {
            discussions = await Task.Run(() => history.ListDiscussions());
            busy = false;
            ShowTitles();
        }
        catch (Exception error) { ShowError(error); }
        finally { busy = false; }
    }

    private void ShowTitles()
    {
        string filter = search.Text.Trim();
        var compare = CultureInfo.GetCultureInfo("fr-FR").CompareInfo;
        object[] matches = discussions
            .Where(discussion => compare.IndexOf(discussion.Title, filter,
                CompareOptions.IgnoreCase | CompareOptions.IgnoreNonSpace) >= 0)
            .ToArray();
        titles.BeginUpdate();
        try
        {
            titles.Items.Clear();
            titles.Items.AddRange(matches);
        }
        finally { titles.EndUpdate(); }

        if (!busy)
            status.Text = matches.Length == 0 ? "Aucune discussion trouvée."
                : matches.Length + (matches.Length == 1 ? " discussion" : " discussions");
    }

    private async Task ExportAsync(Discussion discussion)
    {
        if (busy) return;
        busy = true;
        status.Text = "Export en cours…";
        try
        {
            string filename = await Task.Run(() => history.Export(discussion, downloadFolder()));
            status.Text = "Enregistré dans Téléchargements · " + Path.GetFileName(filename);
        }
        catch (Exception error) { ShowError(error); }
        finally { busy = false; }
    }

    private void ShowError(Exception error)
    {
        status.Text = "Impossible de terminer l’opération.";
        MessageBox.Show(this, error.Message, Text, MessageBoxButtons.OK, MessageBoxIcon.Warning);
    }

    internal static string DownloadsFolder()
    {
        Guid folder = new Guid("374DE290-123F-4565-9164-39C4925E467B");
        IntPtr path;
        try
        {
            Marshal.ThrowExceptionForHR(SHGetKnownFolderPath(ref folder, 0x4000, IntPtr.Zero, out path));
        }
        catch (COMException error)
        {
            throw new IOException("Le dossier Téléchargements est introuvable.", error);
        }
        try { return Marshal.PtrToStringUni(path); }
        finally { Marshal.FreeCoTaskMem(path); }
    }

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern IntPtr SendMessage(IntPtr window, int message, IntPtr wParam, string lParam);

    [DllImport("shell32.dll")]
    private static extern int SHGetKnownFolderPath(ref Guid folder, uint flags, IntPtr token, out IntPtr path);
}

internal sealed class Discussion
{
    internal string Id, Title, Rollout, HistoryMode;
    internal bool Archived, HasMessages;

    public override string ToString()
    {
        return Title + (Archived ? "   ·   Archivée" : "");
    }
}

internal sealed class CodexHistory
{
    private readonly string root;

    internal CodexHistory()
    {
        string codexHome = Environment.GetEnvironmentVariable("CODEX_HOME");
        root = string.IsNullOrWhiteSpace(codexHome)
            ? Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), ".codex")
            : codexHome;
    }

    internal List<Discussion> ListDiscussions()
    {
        string database = FindDatabase("state");
        if (database == null)
            throw new IOException("Aucun historique Codex local trouvé dans " + root + ".");

        using (var db = new SqliteDatabase(database))
        {
            var columns = new HashSet<string>();
            db.Query("PRAGMA table_info(threads)", null, read => columns.Add(read(1)));
            string name = columns.Contains("name") ? "NULLIF(TRIM(name), ''), " : "";
            string mode = columns.Contains("history_mode") ? "history_mode" : "'legacy'";
            string source = columns.Contains("thread_source")
                ? "AND COALESCE(thread_source, 'user') = 'user' " : "";
            string recency = columns.Contains("recency_at_ms")
                ? "COALESCE(NULLIF(recency_at_ms, 0), updated_at * 1000)" : "updated_at";
            string hasMessages = "has_user_event = 1";
            if (columns.Contains("first_user_message")) hasMessages += " OR first_user_message <> ''";
            if (columns.Contains("preview")) hasMessages += " OR preview <> ''";
            var rows = new List<Discussion>();
            db.Query(
                "SELECT id, COALESCE(" + name + "NULLIF(TRIM(title), ''), 'Discussion sans titre'), " +
                "rollout_path, archived, " + mode + ", CASE WHEN " + hasMessages + " THEN 1 ELSE 0 END FROM threads " +
                "WHERE source NOT LIKE '%subagent%' " + source + "ORDER BY " + recency + " DESC, id DESC",
                null, read => rows.Add(new Discussion
                {
                    Id = read(0), Title = Regex.Replace(read(1), @"[\r\n\t]+", " "), Rollout = read(2),
                    Archived = read(3) == "1", HistoryMode = read(4), HasMessages = read(5) == "1"
                }));
            return rows;
        }
    }

    internal string Export(Discussion discussion, string folder)
    {
        Directory.CreateDirectory(folder);
        string temporary = Path.Combine(folder, ".codex-" + Guid.NewGuid().ToString("N") + ".tmp");
        try
        {
            using (var stream = new FileStream(temporary, FileMode.CreateNew, FileAccess.Write, FileShare.None))
            using (var writer = new StreamWriter(stream, new UTF8Encoding(false)))
            {
                writer.NewLine = "\n";
                writer.WriteLine("# " + Regex.Replace(discussion.Title, @"[\\`*_{}\[\]()#+.!<>|~-]", @"\$0"));
                WriteDiscussion(discussion, writer);
            }

            string name = Filename(discussion.Title);
            for (var copy = 1; ; copy++)
            {
                string target = Path.Combine(folder, name + (copy == 1 ? "" : " (" + copy + ")") + ".md");
                try
                {
                    File.Move(temporary, target);
                    return target;
                }
                catch (IOException)
                {
                    if (!File.Exists(target) && !Directory.Exists(target)) throw;
                }
            }
        }
        finally
        {
            if (File.Exists(temporary)) File.Delete(temporary);
        }
    }

    private void WriteDiscussion(Discussion discussion, TextWriter writer)
    {
        var json = new JavaScriptSerializer { MaxJsonLength = int.MaxValue };
        int count = 0;
        string database = discussion.HistoryMode == "legacy" ? null : FindDatabase("thread_history");
        if (database != null)
        {
            using (var db = new SqliteDatabase(database))
            {
                db.Query("SELECT item_json FROM thread_items WHERE thread_id = ? AND " +
                    "item_type IN ('userMessage', 'agentMessage') ORDER BY rollout_ordinal",
                    discussion.Id, read =>
                    {
                        if (WriteMessage(writer, json.DeserializeObject(read(0)) as IDictionary<string, object>))
                            count++;
                    });
            }
        }
        if (count > 0) return;

        string rollout = discussion.Rollout;
        if (!File.Exists(rollout) && !string.IsNullOrEmpty(rollout))
            rollout = Path.Combine(root, "archived_sessions", Path.GetFileName(rollout));
        if (File.Exists(rollout))
        {
            using (var stream = new FileStream(rollout, FileMode.Open, FileAccess.Read, FileShare.ReadWrite | FileShare.Delete))
            using (var reader = new StreamReader(stream, Encoding.UTF8))
            {
                string line;
                while ((line = reader.ReadLine()) != null)
                {
                    if (!line.Contains("\"UserMessage\"") && !line.Contains("\"AgentMessage\"") &&
                        !line.Contains("\"user_message\"") && !line.Contains("\"agent_message\"")) continue;

                    IDictionary<string, object> entry;
                    try { entry = json.DeserializeObject(line) as IDictionary<string, object>; }
                    catch (ArgumentException)
                    {
                        if (!reader.EndOfStream) throw;
                        break;
                    }
                    if (Get(entry, "type") as string != "event_msg") continue;
                    var payload = Get(entry, "payload") as IDictionary<string, object>;
                    var item = Get(payload, "type") as string == "item_completed"
                        ? Get(payload, "item") as IDictionary<string, object> : payload;
                    if (WriteMessage(writer, item)) count++;
                }
            }
        }

        if (count > 0) return;
        if (discussion.HasMessages || (database == null && !File.Exists(rollout)))
            throw new IOException("Les messages de cette discussion sont introuvables dans l’historique local.");
        writer.WriteLine("\n_Aucun message enregistré._");
    }

    private static bool WriteMessage(TextWriter writer, IDictionary<string, object> item)
    {
        string type = Get(item, "type") as string;
        bool user = type == "userMessage" || type == "UserMessage" || type == "user_message";
        if (!user && type != "agentMessage" && type != "AgentMessage" && type != "agent_message") return false;

        var parts = new List<string>();
        string text = Get(item, "text") as string ?? Get(item, "message") as string;
        if (!string.IsNullOrWhiteSpace(text)) parts.Add(text);
        var content = Get(item, "content") as IEnumerable;
        if (content != null)
        {
            foreach (object rawPart in content)
            {
                var part = rawPart as IDictionary<string, object>;
                string kind = Get(part, "type") as string;
                if (string.Equals(kind, "text", StringComparison.OrdinalIgnoreCase))
                {
                    text = Get(part, "text") as string;
                    if (!string.IsNullOrWhiteSpace(text)) parts.Add(text);
                }
                else if (kind != null)
                {
                    parts.Add(kind.IndexOf("image", StringComparison.OrdinalIgnoreCase) >= 0
                        ? "_[Image jointe]_" : "_[Contenu joint]_");
                }
            }
        }
        var images = Get(item, "images") as object[];
        if (images != null && images.Length > 0)
            parts.Add("_[Image jointe]_");
        if (parts.Count == 0) return false;

        string phase = Get(item, "phase") as string;
        writer.WriteLine();
        writer.WriteLine(user ? "## Vous" : phase == "commentary" ? "## Codex · Suivi" : "## Codex");
        writer.WriteLine();
        writer.WriteLine(string.Join("\n\n", parts).Replace("\r\n", "\n").Replace("\r", "\n").TrimEnd('\n'));
        return true;
    }

    private static object Get(IDictionary<string, object> map, string key)
    {
        object value;
        return map != null && map.TryGetValue(key, out value) ? value : null;
    }

    private string FindDatabase(string prefix)
    {
        if (!Directory.Exists(root)) return null;
        var versions = new SortedDictionary<int, string>();
        foreach (string file in Directory.EnumerateFiles(root, prefix + "_*.sqlite"))
        {
            string version = Path.GetFileNameWithoutExtension(file).Substring(prefix.Length + 1);
            int number;
            if (int.TryParse(version, out number)) versions[number] = file;
        }
        return versions.Count == 0 ? null : versions.Last().Value;
    }

    private static string Filename(string title)
    {
        char[] invalid = Path.GetInvalidFileNameChars();
        string name = new string(title.Select(ch => invalid.Contains(ch) ? ' ' : ch).ToArray());
        name = Regex.Replace(name, @"\s+", " ").Trim(' ', '.');
        if (name.Length > 110)
            name = name.Substring(0, char.IsHighSurrogate(name[109]) ? 109 : 110).TrimEnd(' ', '.');
        if (name.Length == 0) return "Discussion Codex";
        if (Regex.IsMatch(name, @"^(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³]|CONIN\$|CONOUT\$)(?:\s*\.|$)",
            RegexOptions.IgnoreCase | RegexOptions.CultureInvariant))
            name = "Codex - " + name;
        return name;
    }
}

internal sealed class SqliteDatabase : IDisposable
{
    private IntPtr handle;

    internal SqliteDatabase(string filename)
    {
        int result = sqlite3_open_v2(Utf8(filename), out handle, 1, IntPtr.Zero);
        if (result != 0)
        {
            var error = Error();
            Dispose();
            throw error;
        }
        sqlite3_busy_timeout(handle, 1500);
    }

    internal void Query(string sql, string parameter, Action<Func<int, string>> row)
    {
        IntPtr statement;
        int result = sqlite3_prepare_v2(handle, Utf8(sql), -1, out statement, IntPtr.Zero);
        if (result != 0) throw Error();
        try
        {
            if (parameter != null && sqlite3_bind_text(statement, 1, Utf8(parameter), -1, new IntPtr(-1)) != 0)
                throw Error();
            while ((result = sqlite3_step(statement)) == 100)
                row(column => ReadText(sqlite3_column_text(statement, column), sqlite3_column_bytes(statement, column)));
            if (result != 101) throw Error();
        }
        finally { sqlite3_finalize(statement); }
    }

    public void Dispose()
    {
        if (handle == IntPtr.Zero) return;
        sqlite3_close(handle);
        handle = IntPtr.Zero;
    }

    private IOException Error()
    {
        IntPtr message = handle == IntPtr.Zero ? IntPtr.Zero : sqlite3_errmsg(handle);
        int bytes = 0;
        if (message != IntPtr.Zero)
            while (Marshal.ReadByte(message, bytes) != 0) bytes++;
        return new IOException("Impossible de lire l’historique Codex. " + ReadText(message, bytes));
    }

    private static byte[] Utf8(string text) { return Encoding.UTF8.GetBytes(text + "\0"); }

    private static string ReadText(IntPtr text, int length)
    {
        if (text == IntPtr.Zero) return null;
        var bytes = new byte[length];
        Marshal.Copy(text, bytes, 0, length);
        return Encoding.UTF8.GetString(bytes);
    }

    private const string Library = "winsqlite3.dll";
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_open_v2(byte[] path, out IntPtr db, int flags, IntPtr vfs);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_close(IntPtr db);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_busy_timeout(IntPtr db, int milliseconds);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern IntPtr sqlite3_errmsg(IntPtr db);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_prepare_v2(IntPtr db, byte[] sql, int length, out IntPtr statement, IntPtr tail);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_bind_text(IntPtr statement, int index, byte[] text, int length, IntPtr destructor);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_step(IntPtr statement);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern IntPtr sqlite3_column_text(IntPtr statement, int column);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_column_bytes(IntPtr statement, int column);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_finalize(IntPtr statement);
}
