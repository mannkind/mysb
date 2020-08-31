using System.Threading;
using System.Threading.Tasks;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using Mysb.Models.Options;
using Mysb.Models.Shared;
using TwoMQTT.Core.Interfaces;
using TwoMQTT.Core.Liasons;

namespace Mysb.Liasons
{
    /// <summary>
    /// I'm just here so I won't be fined.
    /// </summary>
    public class SourceLiason : SourceLiasonBase<object, object, NodeFirmwareInfoMapping, object, SharedOpts>, ISourceLiason<object, object>
    {
        public SourceLiason(ILogger<SourceLiason> logger, IOptions<Models.Options.SharedOpts> sharedOpts) :
            base(logger, new object { }, sharedOpts)
        {
        }

        protected override Task<object?> FetchOneAsync(NodeFirmwareInfoMapping key, CancellationToken cancellationToken) =>
            Task.FromResult<object?>(null);
    }
}